docker-build:
	@echo "==> Build and run the raffle docker container" 
	docker build --tag raffle:latest --build-arg local=true .
	docker run --name raffle -p 8080:8080 -d raffle:latest

docker-stop:
	@echo "==> Stopping and removing the raffle docker container" 
	docker stop raffle
	docker rm raffle

tools: 
	@echo "==> installing required tooling..." 
	go install honnef.co/go/tools/cmd/staticcheck@latest 
	go install mvdan.cc/gofumpt@latest
	go install golang.org/x/tools/cmd/godoc@latest 
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/jstemmer/go-junit-report/v2@latest

test-jenkins:
	@echo "==> Running all tests, and output to junit .xml"
	go test -v -coverprofile coverage.out 2>&1 ./... | go-junit-report -set-exit-code > go_report.xml
	go tool cover -func coverage.out