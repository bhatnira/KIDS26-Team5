data "aws_caller_identity" "current" {}

# The cluster nodes boot from the "hashistack-*" AMI produced by the Packer
# build (image.pkr.hcl) into this same account. Look up the most recent one so
# operators don't have to paste an AMI id after every `packer build`. The lookup
# is skipped entirely when var.ami is set (see outputs.tf), which lets a fresh
# checkout plan against an explicit AMI before any build exists.
data "aws_ami" "hashistack" {
  count       = var.ami == "" ? 1 : 0
  most_recent = true
  owners      = [data.aws_caller_identity.current.account_id]

  filter {
    name   = "name"
    values = ["hashistack-*"]
  }

  filter {
    name   = "architecture"
    values = ["x86_64"]
  }
}

data "aws_availability_zones" "available" {
  state = "available"
}
