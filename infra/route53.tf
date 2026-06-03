data "aws_route53_zone" "selected" {
  count = var.hosted_zone_id == "" && var.hosted_zone_name != "" ? 1 : 0

  name         = var.hosted_zone_name
  private_zone = false
}

locals {
  dns_enabled     = var.hosted_zone_id != "" || var.hosted_zone_name != ""
  dns_zone_name   = trimsuffix(var.hosted_zone_name, ".")
  route53_zone_id = var.hosted_zone_id != "" ? replace(var.hosted_zone_id, "/hostedzone/", "") : (var.hosted_zone_name != "" ? data.aws_route53_zone.selected[0].zone_id : "")

  signaling_dns_name = var.signaling_record_name != "" ? trimsuffix(var.signaling_record_name, ".") : "signal.${local.dns_zone_name}"
  turn_dns_name      = var.turn_record_name != "" ? trimsuffix(var.turn_record_name, ".") : "turn.${local.dns_zone_name}"
}

resource "aws_route53_record" "signaling" {
  count = local.dns_enabled ? 1 : 0

  zone_id         = local.route53_zone_id
  name            = local.signaling_dns_name
  type            = "A"
  ttl             = var.dns_ttl
  records         = [aws_eip.ecs.public_ip]
  allow_overwrite = true
}

resource "aws_route53_record" "turn" {
  count = local.dns_enabled ? 1 : 0

  zone_id         = local.route53_zone_id
  name            = local.turn_dns_name
  type            = "A"
  ttl             = var.dns_ttl
  records         = [aws_eip.ecs.public_ip]
  allow_overwrite = true
}
