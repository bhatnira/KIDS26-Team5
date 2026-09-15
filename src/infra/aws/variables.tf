# ----- Global -----

variable "region" {
  description = "AWS region to deploy the cluster into."
  type        = string
  default     = "us-east-1"
}

variable "key_name" {
  description = "Name of an existing EC2 key pair used for SSH access (user `ubuntu`)."
  type        = string
}

variable "ssh_cidr" {
  description = "CIDR block allowed to SSH into the cluster."
  type        = string
  default     = "0.0.0.0/0"
}

variable "allowlist_ip" {
  description = "CIDR block allowed to reach the Nomad (4646) and Consul (8500) UIs."
  type        = string
  default     = "0.0.0.0/0"
}

variable "vpc_cidr" {
  description = "CIDR block for the cluster VPC."
  type        = string
  default     = "10.100.0.0/16"
}

variable "subnet_cidr" {
  description = "CIDR block for the public subnet inside the VPC."
  type        = string
  default     = "10.100.1.0/24"
}

# ----- Nomad cluster -----

variable "ami" {
  description = "Explicit AMI id for the Nomad nodes. Leave empty to auto-look-up the most recent self-owned `hashistack-*` AMI built by Packer (image.pkr.hcl)."
  type        = string
  default     = ""
}

variable "name_prefix" {
  description = "Prefix for Nomad instance Name tags and the cluster IAM role. Alphanumeric/hyphen only."
  type        = string
  default     = "nomad"
}

variable "server_count" {
  description = "Number of Nomad/Consul servers."
  type        = number
  default     = 3
}

variable "client_count" {
  description = "Number of Nomad clients."
  type        = number
  default     = 3
}

variable "server_instance_type" {
  description = "EC2 instance type for Nomad servers."
  type        = string
  default     = "t3.micro"
}

variable "client_instance_type" {
  description = "EC2 instance type for Nomad clients."
  type        = string
  default     = "t3.small"
}

variable "retry_join" {
  description = "Consul cloud auto-join string used to form the cluster."
  type        = string
  default     = "provider=aws tag_key=ConsulAutoJoin tag_value=auto-join"
}

variable "nomad_binary" {
  description = "Optional URL of a zip containing a nomad binary to replace the one in the AMI. Empty keeps the AMI's version."
  type        = string
  default     = ""
}

variable "root_volume_size_gb" {
  description = "Root EBS volume size in GiB for every cluster node."
  type        = number
  default     = 16
}

variable "client_data_volume_size_gb" {
  description = "Size in GiB of the extra /dev/xvdd scratch volume on client nodes."
  type        = number
  default     = 50
}

# ----- Nextflow / S3 -----

variable "nextflow_bucket_name" {
  description = "Globally-unique S3 bucket name used by Nextflow for outputs (under `data/`) and the work directory (under `work/`)."
  type        = string
}

variable "nextflow_bucket_force_destroy" {
  description = "Allow `terraform destroy` to wipe the bucket even if it still has objects. Useful for ephemeral test runs."
  type        = bool
  default     = true
}

# ----- AWS Batch -----

variable "batch_name_prefix" {
  description = "Prefix applied to Batch IAM roles, launch template, and the job queue name."
  type        = string
  default     = "nextflow"
}

variable "batch_max_vcpus" {
  description = "Maximum vCPUs the spot compute environment will scale to."
  type        = number
  default     = 16
}

variable "batch_instance_types" {
  description = "Instance types Batch can pick from. `optimal` lets Batch choose from the C/M/R families based on job sizing."
  type        = list(string)
  default     = ["optimal"]
}

variable "batch_spot_bid_percentage" {
  description = "Max % of the on-demand price Batch is willing to pay for spot capacity."
  type        = number
  default     = 100
}
