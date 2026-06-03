data "aws_ami" "ecs_optimized" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amzn2-ami-ecs-hvm-*-x86_64-ebs"]
  }
}

resource "aws_security_group" "ecs_host" {
  name        = "${local.name_prefix}-ecs-host"
  description = "Ingress for LylatLink signaling and TURN"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "LylatLink signaling HTTP/WebSocket"
    from_port   = var.signaling_port
    to_port     = var.signaling_port
    protocol    = "tcp"
    cidr_blocks = [var.allowed_cidr]
  }

  ingress {
    description = "TURN TCP"
    from_port   = var.turn_port
    to_port     = var.turn_port
    protocol    = "tcp"
    cidr_blocks = [var.allowed_cidr]
  }

  ingress {
    description = "TURN UDP"
    from_port   = var.turn_port
    to_port     = var.turn_port
    protocol    = "udp"
    cidr_blocks = [var.allowed_cidr]
  }

  ingress {
    description = "TURN UDP relay range"
    from_port   = var.turn_relay_min_port
    to_port     = var.turn_relay_max_port
    protocol    = "udp"
    cidr_blocks = [var.allowed_cidr]
  }

  egress {
    description = "Outbound internet"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${local.name_prefix}-ecs-host-sg"
  }
}

resource "aws_iam_role" "ecs_instance" {
  name = "${local.name_prefix}-ecs-instance"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_instance_ecs" {
  role       = aws_iam_role.ecs_instance.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"
}

resource "aws_iam_role_policy_attachment" "ecs_instance_ssm" {
  role       = aws_iam_role.ecs_instance.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "ecs_instance" {
  name = "${local.name_prefix}-ecs-instance"
  role = aws_iam_role.ecs_instance.name
}

resource "aws_instance" "ecs" {
  ami                         = data.aws_ami.ecs_optimized.id
  instance_type               = var.instance_type
  subnet_id                   = aws_subnet.public.id
  vpc_security_group_ids      = [aws_security_group.ecs_host.id]
  iam_instance_profile        = aws_iam_instance_profile.ecs_instance.name
  associate_public_ip_address = true

  user_data = <<-EOF
    #!/bin/bash
    cat >/etc/ecs/ecs.config <<ECS
    ECS_CLUSTER=${aws_ecs_cluster.main.name}
    ECS_ENABLE_TASK_IAM_ROLE=true
    ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST=true
    ECS
  EOF

  root_block_device {
    volume_size = var.root_volume_size_gb
    volume_type = "gp3"
  }

  tags = {
    Name = "${local.name_prefix}-ecs"
  }
}

resource "aws_eip" "ecs" {
  domain = "vpc"

  tags = {
    Name = "${local.name_prefix}-ecs-eip"
  }
}

resource "aws_eip_association" "ecs" {
  instance_id   = aws_instance.ecs.id
  allocation_id = aws_eip.ecs.id
}

