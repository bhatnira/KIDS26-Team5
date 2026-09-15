output "server_public_ips" {
  description = "Public IPs of the Nomad server nodes."
  value       = aws_instance.server[*].public_ip
}

output "server_private_ips" {
  description = "Private IPs of the Nomad server nodes inside the VPC."
  value       = aws_instance.server[*].private_ip
}

output "client_public_ips" {
  description = "Public IPs of the Nomad client nodes."
  value       = aws_instance.client[*].public_ip
}

output "nomad_addr" {
  description = "Base URL of the Nomad HTTP API / UI (first server)."
  value       = "http://${aws_instance.server[0].public_ip}:4646"
}

output "consul_addr" {
  description = "Base URL of the Consul HTTP API / UI (first server)."
  value       = "http://${aws_instance.server[0].public_ip}:8500"
}

# Exact name kept so post-setup.sh can `terraform output -raw lb_address_consul_nomad`.
output "lb_address_consul_nomad" {
  description = "Base URL of the first server (Consul on :8500, Nomad on :4646)."
  value       = "http://${aws_instance.server[0].public_ip}"
}

output "consul_token_secret" {
  description = "Consul/Nomad bootstrap auto-join token secret. Used by post-setup.sh to fetch the Nomad user token."
  value       = random_uuid.nomad_token.result
  sensitive   = true
}
