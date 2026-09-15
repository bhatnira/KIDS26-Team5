module "data" {
  source = "./modules/data"

  ami = var.ami
}

module "network" {
  source = "./modules/network"

  vpc_cidr     = var.vpc_cidr
  subnet_cidr  = var.subnet_cidr
  az           = module.data.available_az
  ssh_cidr     = var.ssh_cidr
  allowlist_ip = var.allowlist_ip
}

module "nomad" {
  source = "./modules/nomad"

  ami_id                     = module.data.ami_id
  subnet_id                  = module.network.subnet_id
  security_group_ids         = [module.network.security_group_id]
  key_name                   = var.key_name
  region                     = var.region
  name_prefix                = var.name_prefix
  server_count               = var.server_count
  client_count               = var.client_count
  server_instance_type       = var.server_instance_type
  client_instance_type       = var.client_instance_type
  retry_join                 = var.retry_join
  nomad_binary               = var.nomad_binary
  root_volume_size_gb        = var.root_volume_size_gb
  client_data_volume_size_gb = var.client_data_volume_size_gb
}

module "s3" {
  source = "./modules/s3"

  bucket_name   = var.nextflow_bucket_name
  force_destroy = var.nextflow_bucket_force_destroy
}

module "batch" {
  source = "./modules/batch"

  name_prefix         = var.batch_name_prefix
  subnet_ids          = [module.network.subnet_id]
  security_group_ids  = [module.network.security_group_id]
  bucket_arn          = module.s3.bucket_arn
  max_vcpus           = var.batch_max_vcpus
  instance_types      = var.batch_instance_types
  spot_bid_percentage = var.batch_spot_bid_percentage
}
