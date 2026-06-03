output "signaling_url" {
  value       = local.signal_url
  description = "Production signaling URL compiled into clients or used with the development -signal-url override."
}

output "turn_urls" {
  value       = local.turn_urls
  description = "TURN URLs returned to clients by the signaling server."
}

output "signaling_host" {
  value       = local.signal_host
  description = "Hostname or IP clients should use for signaling."
}

output "turn_host" {
  value       = local.turn_host
  description = "Hostname or IP clients should use for TURN."
}

output "signaling_dns_name" {
  value       = local.dns_enabled ? local.signaling_dns_name : null
  description = "Route 53 signaling record name, when DNS is enabled."
}

output "turn_dns_name" {
  value       = local.dns_enabled ? local.turn_dns_name : null
  description = "Route 53 TURN record name, when DNS is enabled."
}

output "ecs_public_ip" {
  value       = aws_eip.ecs.public_ip
  description = "Elastic IP attached to the ECS host."
}

output "signaling_ecr_repository_url" {
  value       = aws_ecr_repository.signaling.repository_url
  description = "ECR repository for the signaling image."
}

output "turn_ecr_repository_url" {
  value       = aws_ecr_repository.turn.repository_url
  description = "ECR repository for the TURN image."
}

output "turn_secret_parameter_name" {
  value       = aws_ssm_parameter.turn_secret.name
  description = "SSM parameter containing the coturn shared secret."
}
