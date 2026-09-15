output "vpc_id" {
  description = "ID of the cluster VPC."
  value       = aws_vpc.this.id
}

output "subnet_id" {
  description = "ID of the public subnet hosting the cluster."
  value       = aws_subnet.public.id
}

output "security_group_id" {
  description = "ID of the cluster security group (shared by Nomad nodes and Batch instances)."
  value       = aws_security_group.cluster.id
}
