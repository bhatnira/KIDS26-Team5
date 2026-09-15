variable "ami_id" {
  description = "AMI to boot the Nomad server and client nodes from (the Packer hashistack image)."
  type        = string
}

variable "subnet_id" {
  description = "Subnet ID the cluster instances launch into."
  type        = string
}

variable "security_group_ids" {
  description = "Security groups attached to every cluster instance. Must allow SSH, Nomad/Consul UI, and intra-cluster traffic."
  type        = list(string)
}

variable "key_name" {
  description = "Name of an existing EC2 key pair for SSH access (user `ubuntu`)."
  type        = string
}

variable "region" {
  description = "AWS region the cluster runs in. Passed to the bootstrap scripts for Consul cloud auto-join."
  type        = string
}

variable "name_prefix" {
  description = "Prefix applied to instance Name tags and the IAM role/profile."
  type        = string
  default     = "nomad"
}

variable "server_count" {
  description = "Number of Nomad/Consul server nodes (used for bootstrap_expect)."
  type        = number
  default     = 3
}

variable "client_count" {
  description = "Number of Nomad client nodes that run workloads."
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
  description = "Consul cloud auto-join string. Discovers peers by the ConsulAutoJoin instance tag."
  type        = string
  default     = "provider=aws tag_key=ConsulAutoJoin tag_value=auto-join"
}

variable "nomad_binary" {
  description = "Optional URL of a zip containing a nomad binary to replace the one baked into the AMI. Empty keeps the AMI's version."
  type        = string
  default     = ""
}

variable "root_volume_size_gb" {
  description = "Root EBS volume size in GiB for every cluster node."
  type        = number
  default     = 16
}

variable "client_data_volume_size_gb" {
  description = "Size in GiB of the extra /dev/xvdd scratch volume attached to client nodes."
  type        = number
  default     = 50
}
