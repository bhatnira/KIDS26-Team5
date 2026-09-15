output "bucket_id" {
  description = "S3 bucket name."
  value       = module.bucket.s3_bucket_id
}

output "bucket_arn" {
  description = "S3 bucket ARN."
  value       = module.bucket.s3_bucket_arn
}

output "bucket_region" {
  description = "Region the bucket lives in."
  value       = module.bucket.s3_bucket_region
}

output "data_uri" {
  description = "s3:// URI to hand to Nextflow's --outdir."
  value       = "s3://${module.bucket.s3_bucket_id}/data"
}

output "work_uri" {
  description = "s3:// URI to hand to Nextflow's -work-dir."
  value       = "s3://${module.bucket.s3_bucket_id}/work"
}
