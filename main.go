package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

type PurchaseRequest struct {
	CompetitionID int    `json:"competition_id"`
	TicketQty     int    `json:"ticket_qty"`
	ClientSeed    string `json:"client_seed"`
	CustomerName  string `json:"customer_name"`
	MobileNumber  string `json:"mobile_number"`
	Email         string `json:"email"`
	PaymentMethod string `json:"payment_method"`
	PaymentStatus string `json:"payment_status"`
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func main() {
	rand.Seed(time.Now().UnixNano())

	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "secret")
	dbName := getEnv("DB_NAME", "raffle")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable", dbUser, dbPassword, dbName, dbHost, dbPort)

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer db.Close()

	// Retry connection loop for container startup synchronization
	for i := 0; i < 10; i++ {
		err = db.Ping()
		if err == nil {
			break
		}
		log.Printf("Waiting for database connection... (%d/10)", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Could not connect to PostgreSQL after retries: %v", err)
	}

	initDBSchema()

	http.HandleFunc("/api/purchase", purchaseHandler)
	http.HandleFunc("/api/verify-ticket", verifyTicketHandler)
	http.HandleFunc("/api/admin/status", adminStatusHandler)
	http.HandleFunc("/api/admin/close-competition", closeCompetitionHandler)
	http.HandleFunc("/api/admin/lock-entries", lockEntriesHandler)
	http.HandleFunc("/api/admin/draw", executeDrawHandler)
	http.HandleFunc("/api/regulator/audit-logs", auditLogsHandler)

	handler := enableCors(http.DefaultServeMux)

	fmt.Println("🚀 Comprehensive Raffle Regulatory PoC Server Running on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

func enableCors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func initDBSchema() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS competitions (
			id SERIAL PRIMARY KEY,
			title VARCHAR(255) NOT NULL,
			ticket_price INT NOT NULL,
			total_tickets INT NOT NULL,
			status VARCHAR(50) DEFAULT 'ACTIVE',
			locked BOOLEAN DEFAULT FALSE,
			winner_ticket INT DEFAULT NULL,
			closing_date TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS raffle_entries (
			id SERIAL PRIMARY KEY,
			competition_id INT,
			session_id VARCHAR(64) UNIQUE NOT NULL,
			customer_name VARCHAR(255),
			mobile_number VARCHAR(50),
			email VARCHAR(255),
			ticket_qty INT NOT NULL,
			ticket_range VARCHAR(128) NOT NULL,
			payment_ref VARCHAR(64),
			payment_status VARCHAR(50),
			client_seed VARCHAR(64),
			unhashed_server_seed VARCHAR(128),
			hashed_server_seed VARCHAR(128),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id SERIAL PRIMARY KEY,
			action_time VARCHAR(20),
			action_desc TEXT
		);`,
	}

	for _, q := range queries {
		_, err := db.Exec(q)
		if err != nil {
			log.Fatalf("Schema initialization error: %v", err)
		}
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM competitions").Scan(&count)
	if count == 0 {
		db.Exec(`INSERT INTO competitions (title, ticket_price, total_tickets, status, locked, closing_date) 
		         VALUES ('Win a Toyota Land Cruiser', 10000, 1000, 'ACTIVE', false, $1)`, time.Now().Add(7*24*time.Hour))
		logAudit("Competition created: Win a Toyota Land Cruiser")
	}
	fmt.Println("✅ Database schema and seeding complete.")
}

func logAudit(desc string) {
	timeStr := time.Now().Format("15:04")
	db.Exec("INSERT INTO audit_logs (action_time, action_desc) VALUES ($1, $2)", timeStr, desc)
}

func sha256String(input string) string {
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}

func purchaseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, _ := ioutil.ReadAll(r.Body)
	var req PurchaseRequest
	json.Unmarshal(bodyBytes, &req)

	qty := req.TicketQty
	if qty <= 0 || qty > 999 {
		http.Error(w, "Invalid ticket quantity. Maximum allowed is 999 per transaction.", http.StatusBadRequest)
		return
	}

	var status string
	var locked bool
	db.QueryRow("SELECT status, locked FROM competitions WHERE id = $1", req.CompetitionID).Scan(&status, &locked)
	if status != "ACTIVE" || locked {
		http.Error(w, "Competition is closed or entries are locked.", http.StatusBadRequest)
		return
	}

	sessionID := fmt.Sprintf("SESS-%s", generateRandomString(8))
	paymentRef := fmt.Sprintf("PAY-%s", generateRandomString(10))
	startTicket := rand.Intn(800000) + 100000
	endTicket := startTicket + qty - 1
	ticketRange := fmt.Sprintf("#%04d – #%04d (%d tickets)", startTicket, endTicket, qty)

	unhashedServerSeed := fmt.Sprintf("SRV-SEC-%s", generateRandomString(12))
	hashedServerSeed := sha256String(unhashedServerSeed)

	_, err := db.Exec(`
		INSERT INTO raffle_entries 
		(competition_id, session_id, customer_name, mobile_number, email, ticket_qty, ticket_range, payment_ref, payment_status, client_seed, unhashed_server_seed, hashed_server_seed)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, req.CompetitionID, sessionID, req.CustomerName, req.MobileNumber, req.Email, qty, ticketRange, paymentRef, req.PaymentStatus, req.ClientSeed, unhashedServerSeed, hashedServerSeed)

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if req.PaymentStatus == "SUCCESS" {
		logAudit(fmt.Sprintf("Customer %s entered competition. Payment successful. Tickets issued: %s", req.CustomerName, ticketRange))
	} else {
		logAudit(fmt.Sprintf("Customer %s attempted entry. Payment FAILED. Recorded.", req.CustomerName))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       req.PaymentStatus == "SUCCESS",
		"session_id":    sessionID,
		"payment_ref":   paymentRef,
		"ticket_range":  ticketRange,
		"hashed_seed":   hashedServerSeed,
		"unhashed_seed": unhashedServerSeed,
		"timestamp":     time.Now().Format("2006-01-02 15:04:05"),
	})
}

func verifyTicketHandler(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	var sessionID, ticketRange, paymentStatus, customerName string
	var createdAt time.Time

	err := db.QueryRow(`
		SELECT session_id, ticket_range, payment_status, customer_name, created_at 
		FROM raffle_entries WHERE session_id = $1 OR payment_ref = $1
	`, ref).Scan(&sessionID, &ticketRange, &paymentStatus, &customerName, &createdAt)

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"found": false})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"found":          true,
		"session_id":     sessionID,
		"ticket_range":   ticketRange,
		"payment_status": paymentStatus,
		"customer_name":  customerName,
		"created_at":     createdAt.Format("2006-01-02 15:04:05"),
	})
}

func adminStatusHandler(w http.ResponseWriter, r *http.Request) {
	var totalTickets, soldTickets int
	db.QueryRow("SELECT total_tickets FROM competitions WHERE id = 1").Scan(&totalTickets)
	db.QueryRow("SELECT COALESCE(SUM(ticket_qty), 0) FROM raffle_entries WHERE payment_status = 'SUCCESS'").Scan(&soldTickets)

	var status string
	var locked bool
	var winnerTicket sql.NullInt64
	db.QueryRow("SELECT status, locked, winner_ticket FROM competitions WHERE id = 1").Scan(&status, &locked, &winnerTicket)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_tickets":  totalTickets,
		"tickets_sold":   soldTickets,
		"remaining":      totalTickets - soldTickets,
		"status":         status,
		"locked":         locked,
		"winner_ticket":  winnerTicket.Valid,
		"winning_ticket": winnerTicket.Int64,
	})
}

func closeCompetitionHandler(w http.ResponseWriter, r *http.Request) {
	db.Exec("UPDATE competitions SET status = 'CLOSED' WHERE id = 1")
	logAudit("Competition closed by operator.")
	w.WriteHeader(http.StatusOK)
}

func lockEntriesHandler(w http.ResponseWriter, r *http.Request) {
	db.Exec("UPDATE competitions SET locked = true WHERE id = 1")
	logAudit("Entry list locked permanently. Immutable state active.")
	w.WriteHeader(http.StatusOK)
}

func executeDrawHandler(w http.ResponseWriter, r *http.Request) {
	logAudit("Random draw started.")
	rows, err := db.Query("SELECT unhashed_server_seed, client_seed, ticket_range FROM raffle_entries WHERE payment_status = 'SUCCESS'")
	if err != nil {
		http.Error(w, "Failed to load entries", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type EntryData struct {
		srvSeed string
		cliSeed string
		rng     string
	}
	var entries []EntryData
	for rows.Next() {
		var srv, cli, rng string
		rows.Scan(&srv, &cli, &rng)
		entries = append(entries, EntryData{srvSeed: srv, cliSeed: cli, rng: rng})
	}

	if len(entries) == 0 {
		http.Error(w, "No valid entries available for draw", http.StatusBadRequest)
		return
	}

	selected := entries[0]
	combined := selected.srvSeed + selected.cliSeed
	hash := sha256String(combined)
	var numericVal int64
	fmt.Sscanf(hash[:8], "%x", &numericVal)
	winningTicket := 100000 + (numericVal % 1000)

	db.Exec("UPDATE competitions SET winner_ticket = $1 WHERE id = 1", winningTicket)
	logAudit(fmt.Sprintf("Random draw executed. Ticket #%04d selected as winner.", winningTicket))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"winning_ticket": winningTicket,
		"rng_reference":  hash,
		"timestamp":      time.Now().Format("2006-01-02 15:04:05"),
	})
}

func auditLogsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT action_time, action_desc FROM audit_logs ORDER BY id ASC")
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type LogItem struct {
		Time string `json:"time"`
		Desc string `json:"desc"`
	}
	var logs []LogItem
	for rows.Next() {
		var l LogItem
		rows.Scan(&l.Time, &l.Desc)
		logs = append(logs, l)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func generateRandomString(length int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}