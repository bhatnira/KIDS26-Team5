variable "name_prefix" {
  description = "Prefix applied to Batch IAM roles, the launch template, and the job queue name."
  type        = string
  default     = "cab-test"
}

variable "subnet_ids" {
  description = "Subnets the Batch compute environment will launch instances into."
  type        = list(string)
}

variable "security_group_ids" {
  description = "Security groups attached to Batch compute instances. Must allow egress to S3, ECR, and the Batch API."
  type        = list(string)
}

variable "bucket_arn" {
  description = "ARN of the Nextflow workflow bucket. The compute instance role gets read/write access to it."
  type        = string
}

variable "max_vcpus" {
  description = "Maximum vCPUs the spot compute environment will scale to."
  type        = number
  default     = 16
}

variable "instance_types" {
  description = "Instance types Batch can pick from. `optimal` lets Batch choose from the C/M/R families based on job sizing."
  type        = list(string)
  default     = ["optimal"]
}

variable "spot_bid_percentage" {
  description = "Max % of the on-demand price Batch is willing to pay for spot capacity (1-100)."
  type        = number
  default     = 100
}

variable "tags" {
  description = "Additional tags applied to Batch resources."
  type        = map(string)
  default     = {}
}
