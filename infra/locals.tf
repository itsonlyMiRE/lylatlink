data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  name_prefix = "${var.project_name}-${var.environment}"
  az          = var.availability_zone != "" ? var.availability_zone : data.aws_availability_zones.available.names[0]

  ecr_registry        = split("/", aws_ecr_repository.signaling.repository_url)[0]
  aws_cli_profile_arg = var.aws_profile != "" ? "--profile ${var.aws_profile}" : ""

  manual_public_host = var.public_hostname != "" ? var.public_hostname : aws_eip.ecs.public_ip
  manual_signal_host = var.signaling_hostname != "" ? var.signaling_hostname : local.manual_public_host
  manual_turn_host   = var.turn_hostname != "" ? var.turn_hostname : local.manual_public_host
  signal_host        = local.dns_enabled ? local.signaling_dns_name : local.manual_signal_host
  turn_host          = local.dns_enabled ? local.turn_dns_name : local.manual_turn_host
  signal_url         = "http://${local.signal_host}:${var.signaling_port}"
  turn_urls = [
    "turn:${local.turn_host}:${var.turn_port}?transport=udp",
    "turn:${local.turn_host}:${var.turn_port}?transport=tcp",
  ]

  tags = {
    Project     = var.project_name
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}
