terraform {
  required_version = "1.2.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "= 4.24.0"
    }
  }
  backend "s3" {
    bucket         = "pl-thunderdome-terraform"
    key            = "main.state"
    dynamodb_table = "terraform"
    region         = "eu-west-1"
  }
}

provider "aws" {
  region = "eu-west-1"
}

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

module "vpc" {
  source = "terraform-aws-modules/vpc/aws"

  name = "thunderdome"
  cidr = "10.0.0.0/16"

  azs             = ["eu-west-1a"]
  private_subnets = ["10.0.1.0/24"]
  public_subnets  = ["10.0.100.0/24"]

  enable_ipv6                                    = true
  assign_ipv6_address_on_creation                = true
  private_subnet_assign_ipv6_address_on_creation = false

  public_subnet_ipv6_prefixes  = [0, 1]
  private_subnet_ipv6_prefixes = [2, 3]

  enable_nat_gateway  = true
  single_nat_gateway  = false
  reuse_nat_ips       = true
  external_nat_ip_ids = aws_eip.nat.*.id
}

resource "aws_eip" "nat" {
  count = 3
  vpc   = true
}

module "ecs" {
  source = "terraform-aws-modules/ecs/aws"

  cluster_name = "thunderdome"

  cluster_configuration = {
    execute_command_configuration = {
      logging = "OVERRIDE"
      log_configuration = {
        cloud_watch_log_group_name = "/aws/ecs/aws-ec2"
      }
    }
  }

  fargate_capacity_providers = {
    FARGATE = {
      default_capacity_provider_strategy = {
        weight = 50
      }
    }
  }
}

resource "aws_ecr_repository" "kubo" {
  name = "kubo"
}
