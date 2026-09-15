# ----- Nomad cluster -----

output "nomad_addr" {
  description = "Base URL of the Nomad HTTP API / UI. Set NOMAD_ADDR to this."
  value       = module.nomad.nomad_addr
}

output "consul_addr" {
  description = "Base URL of the Consul HTTP API / UI."
  value       = module.nomad.consul_addr
}

output "server_public_ips" {
  description = "Public IPs of the Nomad server nodes."
  value       = module.nomad.server_public_ips
}

output "client_public_ips" {
  description = "Public IPs of the Nomad client nodes."
  value       = module.nomad.client_public_ips
}

# Consumed by post-setup.sh via `terraform output -raw lb_address_consul_nomad`.
output "lb_address_consul_nomad" {
  description = "Base URL of the first server (Consul :8500, Nomad :4646)."
  value       = module.nomad.lb_address_consul_nomad
}

output "consul_token_secret" {
  description = "Consul/Nomad bootstrap auto-join token secret."
  value       = module.nomad.consul_token_secret
  sensitive   = true
}

output "ssh_to_server" {
  description = "Convenience SSH command to the first Nomad server (hashistack AMI is Ubuntu)."
  value       = "ssh -i ${var.key_name}.pem ubuntu@${module.nomad.server_public_ips[0]}"
}

output "cluster_access" {
  description = "How to reach the running cluster."
  value       = <<-EOT

    Nomad UI:  ${module.nomad.nomad_addr}/ui
    Consul UI: ${module.nomad.consul_addr}/ui

    Server public IPs: ${join(", ", module.nomad.server_public_ips)}
    Client public IPs: ${join(", ", module.nomad.client_public_ips)}

    Run ./post-setup.sh to fetch the Nomad user token, then:
      export NOMAD_ADDR=$(terraform output -raw lb_address_consul_nomad):4646
      export NOMAD_TOKEN=$(cat nomad.token)
  EOT
}

# ----- Nextflow / Batch -----

output "nextflow_bucket" {
  description = "S3 bucket name used by Nextflow (data + work directories live under prefixes)."
  value       = module.s3.bucket_id
}

output "nextflow_outdir" {
  description = "s3:// URI to pass to Nextflow as --outdir."
  value       = module.s3.data_uri
}

output "nextflow_workdir" {
  description = "s3:// URI to pass to Nextflow as -work-dir."
  value       = module.s3.work_uri
}

output "batch_job_queue" {
  description = "AWS Batch job queue name. Set `process.queue` in nextflow.config to this."
  value       = module.batch.job_queue_name
}

output "batch_region" {
  description = "Region the Batch queue and S3 bucket live in. Set `aws.region` in nextflow.config to this."
  value       = var.region
}
