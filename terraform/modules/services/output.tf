output "epr-mq-host" {
  value       = aws_instance.epr-services-server.public_ip
  description = "The address for the MQ"
  sensitive   = false
}
output "epr-mq-port" {
  value       = "5672"
  description = "The port for the MQ"
  sensitive   = false
}
output "epr-mq-username" {
  value       = "guest"
  description = "The username for the MQ"
  sensitive   = true
}
output "epr-mq-password" {
  value       = "guest"
  description = "The password for the MQ"
  sensitive   = true
}
output "epr-mq-queuename" {
  value       = "MessageQueue"
  description = "The Queue Name for the MQ"
  sensitive   = false
}


