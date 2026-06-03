variable "aws_region" {
  type        = string
  description = "AWS region to deploy into."
  default     = "us-east-2"
}

variable "aws_profile" {
  type        = string
  description = "Local AWS CLI profile Terraform and null_resource builds should use."
  default     = null
}

variable "project_name" {
  type        = string
  description = "Name prefix for AWS resources."
  default     = "lylatlink"
}

variable "environment" {
  type        = string
  description = "Deployment environment name."
  default     = "prod"
}

variable "vpc_cidr" {
  type        = string
  description = "VPC CIDR."
  default     = "10.42.0.0/16"
}

variable "public_subnet_cidr" {
  type        = string
  description = "Public subnet CIDR."
  default     = "10.42.1.0/24"
}

variable "availability_zone" {
  type        = string
  description = "Optional AZ for the single public subnet. Empty uses the region's first AZ."
  default     = ""
}

variable "allowed_cidr" {
  type        = string
  description = "CIDR allowed to reach signaling and TURN."
  default     = "0.0.0.0/0"
}

variable "instance_type" {
  type        = string
  description = "ECS container instance type."
  default     = "t3.small"
}

variable "root_volume_size_gb" {
  type        = number
  description = "ECS host root volume size."
  default     = 30
}

variable "signaling_port" {
  type        = number
  description = "Public signaling HTTP/WebSocket port."
  default     = 8787
}

variable "turn_port" {
  type        = number
  description = "Public TURN listening port."
  default     = 3478
}

variable "turn_relay_min_port" {
  type        = number
  description = "Lowest UDP relay port coturn may allocate."
  default     = 49160
}

variable "turn_relay_max_port" {
  type        = number
  description = "Highest UDP relay port coturn may allocate."
  default     = 49200
}

variable "turn_realm" {
  type        = string
  description = "coturn realm."
  default     = "lylatlink"
}

variable "turn_ttl_seconds" {
  type        = number
  description = "TTL for generated TURN credentials returned by signaling."
  default     = 480
}

variable "turn_secret_value" {
  type        = string
  description = "Optional TURN shared secret. If empty, Terraform generates one and stores it in SSM."
  default     = ""
  sensitive   = true
}

variable "public_hostname" {
  type        = string
  description = "Optional shared public DNS hostname to advertise in client URLs instead of the EC2 Elastic IP when managed DNS is disabled. Prefer signaling_hostname and turn_hostname when they differ."
  default     = ""
}

variable "signaling_hostname" {
  type        = string
  description = "Optional externally managed signaling hostname to advertise when Route 53 DNS is disabled."
  default     = ""
}

variable "turn_hostname" {
  type        = string
  description = "Optional externally managed TURN hostname to advertise when Route 53 DNS is disabled."
  default     = ""
}

variable "hosted_zone_id" {
  type        = string
  description = "Optional Route 53 hosted zone ID for creating signaling and TURN DNS records."
  default     = ""
}

variable "hosted_zone_name" {
  type        = string
  description = "Optional authoritative or delegated Route 53 hosted zone name, such as lylatlink.example.com. Used to look up the zone when hosted_zone_id is empty and to derive default record names."
  default     = ""
}

variable "signaling_record_name" {
  type        = string
  description = "Optional FQDN for the signaling A record. Empty defaults to signal.<hosted_zone_name> when DNS is enabled."
  default     = ""
}

variable "turn_record_name" {
  type        = string
  description = "Optional FQDN for the TURN A record. Empty defaults to turn.<hosted_zone_name> when DNS is enabled."
  default     = ""
}

variable "dns_ttl" {
  type        = number
  description = "TTL in seconds for Route 53 A records."
  default     = 300
}

variable "image_tag" {
  type        = string
  description = "Image tag to build/push/deploy."
  default     = "latest"
}

variable "build_images" {
  type        = bool
  description = "Build and push Docker images locally during terraform apply."
  default     = true
}
