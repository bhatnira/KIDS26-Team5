variable "ami" {
  description = "Explicit AMI id for the Nomad server/client nodes. Leave empty to auto-look-up the most recent self-owned `hashistack-*` AMI built by Packer."
  type        = string
  default     = ""
}
