terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }

  # Uncomment and populate to switch from local state to a shared S3 backend.
  # backend "s3" {
  #   bucket         = "nomad-cluster-tfstate"
  #   key            = "infra/aws/terraform.tfstate"
  #   region         = "us-east-1"
  #   dynamodb_table = "nomad-cluster-tflock"
  #   encrypt        = true
  # }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = "nixflow"
      ManagedBy = "terraform"
      Cluster   = "nomad"
    }
  }
}
