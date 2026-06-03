resource "random_password" "turn_secret" {
  length  = 32
  special = false
}

resource "aws_ssm_parameter" "turn_secret" {
  name        = "/${local.name_prefix}/turn/static-auth-secret"
  description = "coturn static auth secret for LylatLink"
  type        = "SecureString"
  value       = var.turn_secret_value != "" ? var.turn_secret_value : random_password.turn_secret.result
}

