output "raffle-test-hostname" {
  value       = aws_db_instance.raffle-test-pgsql.address
  description = "The hostname of the PostgreSQL database"
}

output "raffle-test-port" {
  value       = aws_db_instance.raffle-test-pgsql.port
  description = "The port of the PostgreSQL database"
}

output "raffle-test-username" {
  value       = aws_db_instance.raffle-test-pgsql.username
  description = "The username to connect to PostgreSQL"
}

output "raffle-test-password" {
  value       = aws_db_instance.raffle-test-pgsql.password
  description = "The password to connect to PostgreSQL"
  sensitive   = true
}

output "ecr_repository_url" {
  value       = aws_ecr_repository.raffle_repo.repository_url
  description = "The URL of the ECR repository"
}