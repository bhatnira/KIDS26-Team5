variable "bucket_name" {
  description = "Name of the S3 bucket used by Nextflow for outputs and the work directory. Must be globally unique across AWS."
  type        = string
}

variable "force_destroy" {
  description = "Allow `terraform destroy` to delete the bucket even if it still contains objects. Nextflow work dirs can be huge, so default to true for this ephemeral test infra."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Additional tags applied to the bucket."
  type        = map(string)
  default     = {}
}
