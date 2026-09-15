output "ami_id" {
  description = "AMI the cluster boots from: the explicit override if var.ami is set, otherwise the latest self-owned hashistack-* image."
  value       = var.ami != "" ? var.ami : data.aws_ami.hashistack[0].id
}

output "available_az" {
  description = "First availability zone reported as available in the region."
  value       = data.aws_availability_zones.available.names[0]
}
