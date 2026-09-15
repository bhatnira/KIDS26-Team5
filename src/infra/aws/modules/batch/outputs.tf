output "job_queue_name" {
  description = "Name of the Batch job queue Nextflow should submit jobs to."
  value       = aws_batch_job_queue.nextflow.name
}

output "job_queue_arn" {
  description = "ARN of the Batch job queue."
  value       = aws_batch_job_queue.nextflow.arn
}

output "compute_environment_arn" {
  description = "ARN of the spot compute environment backing the queue."
  value       = module.batch.compute_environments["spot"].arn
}

output "instance_role_arn" {
  description = "ARN of the IAM role attached to Batch compute instances (has read/write access to the workflow bucket)."
  value       = module.batch.instance_iam_role_arn
}
