variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
}

variable "subnet_cidr" {
  description = "CIDR block for the public subnet."
  type        = string
}

variable "az" {
  description = "Availability zone to place the subnet in."
  type        = string
}

variable "ssh_cidr" {
  description = "CIDR block allowed to SSH into the cluster."
  type        = string
}

variable "allowlist_ip" {
  description = "CIDR block allowed to reach the Nomad (4646) and Consul (8500) HTTP APIs / UIs."
  type        = string
}
