resource "aws_ecs_cluster" "main" {
  name = local.name_prefix
}

resource "aws_iam_role" "task_execution" {
  name = "${local.name_prefix}-task-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "task_execution_ssm" {
  name = "${local.name_prefix}-task-execution-ssm"
  role = aws_iam_role.task_execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "ssm:GetParameter",
        "ssm:GetParameters"
      ]
      Resource = aws_ssm_parameter.turn_secret.arn
    }]
  })
}

resource "aws_ecs_task_definition" "signaling" {
  family                   = "${local.name_prefix}-signaling"
  requires_compatibilities = ["EC2"]
  network_mode             = "host"
  execution_role_arn       = aws_iam_role.task_execution.arn
  cpu                      = 128
  memory                   = 256

  container_definitions = jsonencode([{
    name      = "signaling"
    image     = "${aws_ecr_repository.signaling.repository_url}:${var.image_tag}"
    essential = true
    environment = [
      { name = "PORT", value = tostring(var.signaling_port) },
      { name = "TURN_URLS", value = join(",", local.turn_urls) },
      { name = "TURN_TTL_SECONDS", value = tostring(var.turn_ttl_seconds) },
      { name = "LYLATLINK_IMAGE_HASH", value = local.signaling_image_hash }
    ]
    secrets = [
      { name = "TURN_SECRET", valueFrom = aws_ssm_parameter.turn_secret.arn }
    ]
    portMappings = [{
      containerPort = var.signaling_port
      hostPort      = var.signaling_port
      protocol      = "tcp"
    }]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-region        = var.aws_region
        awslogs-group         = aws_cloudwatch_log_group.signaling.name
        awslogs-stream-prefix = "signaling"
      }
    }
  }])

  depends_on = [null_resource.build_signaling_image]
}

resource "aws_ecs_task_definition" "turn" {
  family                   = "${local.name_prefix}-turn"
  requires_compatibilities = ["EC2"]
  network_mode             = "host"
  execution_role_arn       = aws_iam_role.task_execution.arn
  cpu                      = 128
  memory                   = 256

  container_definitions = jsonencode([{
    name      = "turn"
    image     = "${aws_ecr_repository.turn.repository_url}:${var.image_tag}"
    essential = true
    environment = [
      { name = "LYLATLINK_IMAGE_HASH", value = local.turn_image_hash }
    ]
    command = [
      "sh",
      "-c",
      "exec turnserver --log-file=- --no-cli --fingerprint --lt-cred-mech --use-auth-secret --static-auth-secret=\"$TURN_SECRET\" --realm=\"${var.turn_realm}\" --listening-port=${var.turn_port} --min-port=${var.turn_relay_min_port} --max-port=${var.turn_relay_max_port} --external-ip=${aws_eip.ecs.public_ip} --no-tls --no-dtls"
    ]
    secrets = [
      { name = "TURN_SECRET", valueFrom = aws_ssm_parameter.turn_secret.arn }
    ]
    portMappings = [
      {
        containerPort = var.turn_port
        hostPort      = var.turn_port
        protocol      = "tcp"
      },
      {
        containerPort = var.turn_port
        hostPort      = var.turn_port
        protocol      = "udp"
      }
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-region        = var.aws_region
        awslogs-group         = aws_cloudwatch_log_group.turn.name
        awslogs-stream-prefix = "turn"
      }
    }
  }])

  depends_on = [null_resource.build_turn_image]
}

resource "aws_ecs_service" "signaling" {
  name            = "signaling"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.signaling.arn
  desired_count   = 1
  launch_type     = "EC2"

  depends_on = [aws_eip_association.ecs]
}

resource "aws_ecs_service" "turn" {
  name            = "turn"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.turn.arn
  desired_count   = 1
  launch_type     = "EC2"

  depends_on = [aws_eip_association.ecs]
}
