resource "aws_ecr_repository" "signaling" {
  name                 = "${local.name_prefix}-signaling"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_repository" "turn" {
  name                 = "${local.name_prefix}-turn"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "null_resource" "build_signaling_image" {
  count = var.build_images ? 1 : 0

  triggers = {
    image_tag  = var.image_tag
    image_hash = local.signaling_image_hash
  }

  provisioner "local-exec" {
    working_dir = path.module
    command     = <<-EOT
      set -euo pipefail
      export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
      aws ${local.aws_cli_profile_arg} ecr get-login-password --region ${var.aws_region} | podman login --username AWS --password-stdin ${local.ecr_registry}
      podman build --platform linux/amd64 -f docker/Dockerfile.signaling -t ${aws_ecr_repository.signaling.repository_url}:${var.image_tag} ..
      podman push ${aws_ecr_repository.signaling.repository_url}:${var.image_tag}
    EOT
    interpreter = ["/bin/bash", "-c"]
  }
}

resource "null_resource" "build_turn_image" {
  count = var.build_images ? 1 : 0

  triggers = {
    image_tag  = var.image_tag
    image_hash = local.turn_image_hash
  }

  provisioner "local-exec" {
    working_dir = path.module
    command     = <<-EOT
      set -euo pipefail
      export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
      aws ${local.aws_cli_profile_arg} ecr get-login-password --region ${var.aws_region} | podman login --username AWS --password-stdin ${local.ecr_registry}
      podman build --platform linux/amd64 -f docker/Dockerfile.turn -t ${aws_ecr_repository.turn.repository_url}:${var.image_tag} ..
      podman push ${aws_ecr_repository.turn.repository_url}:${var.image_tag}
    EOT
    interpreter = ["/bin/bash", "-c"]
  }
}
