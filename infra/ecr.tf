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
    image_tag       = var.image_tag
    dockerfile_hash = filesha256("${path.module}/../Dockerfile.signaling")
    server_hash     = sha256(join("", [for f in fileset("${path.module}/../server", "**") : filesha256("${path.module}/../server/${f}")]))
  }

  provisioner "local-exec" {
    working_dir = path.module
    command     = <<-EOT
      set -euo pipefail
      export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
      aws ${local.aws_cli_profile_arg} ecr get-login-password --region ${var.aws_region} | podman login --username AWS --password-stdin ${local.ecr_registry}
      podman build --platform linux/amd64 -f ../Dockerfile.signaling -t ${aws_ecr_repository.signaling.repository_url}:${var.image_tag} ..
      podman push ${aws_ecr_repository.signaling.repository_url}:${var.image_tag}
    EOT
    interpreter = ["/bin/bash", "-c"]
  }
}

resource "null_resource" "build_turn_image" {
  count = var.build_images ? 1 : 0

  triggers = {
    image_tag       = var.image_tag
    dockerfile_hash = filesha256("${path.module}/../Dockerfile.turn")
  }

  provisioner "local-exec" {
    working_dir = path.module
    command     = <<-EOT
      set -euo pipefail
      export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
      aws ${local.aws_cli_profile_arg} ecr get-login-password --region ${var.aws_region} | podman login --username AWS --password-stdin ${local.ecr_registry}
      podman build --platform linux/amd64 -f ../Dockerfile.turn -t ${aws_ecr_repository.turn.repository_url}:${var.image_tag} ..
      podman push ${aws_ecr_repository.turn.repository_url}:${var.image_tag}
    EOT
    interpreter = ["/bin/bash", "-c"]
  }
}
