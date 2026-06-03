resource "aws_cloudwatch_log_group" "signaling" {
  name              = "/ecs/${local.name_prefix}/signaling"
  retention_in_days = 14
}

resource "aws_cloudwatch_log_group" "turn" {
  name              = "/ecs/${local.name_prefix}/turn"
  retention_in_days = 14
}

