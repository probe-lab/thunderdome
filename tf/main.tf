terraform {
  required_version = ">= 1.6.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 4"
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
data "aws_kms_key" "default_secretsmanager_key" {
  key_id = "arn:aws:kms:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:key/d32eafb8-e9b3-44f2-9703-fd4663949020"
}

data "aws_secretsmanager_secret" "prometheus-secret" {
  arn = "arn:aws:secretsmanager:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:secret:prometheus_credentials-ujqWDl"
}

data "aws_secretsmanager_secret" "dealgood-loki-secret" {
  arn = "arn:aws:secretsmanager:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:secret:dealgood-loki-aQaPqD"
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.5.1"

  name = "thunderdome"
  cidr = "10.0.0.0/16"

  azs             = ["eu-west-1a"]
  private_subnets = ["10.0.1.0/24"]
  public_subnets  = ["10.0.100.0/24"]

  enable_ipv6 = false # This is mostly historic coincidence as we started out with it enabled

  # we don't assign ipv6 addresses by default as we can't block internal ipv6 traffic with a NACL
  public_subnet_assign_ipv6_address_on_creation  = false
  private_subnet_assign_ipv6_address_on_creation = false

  # Need both of these to make DNS auto-discovery work
  # see https://docs.aws.amazon.com/vpc/latest/userguide/vpc-dns.html#vpc-private-hosted-zones
  enable_dns_hostnames = true
  enable_dns_support   = true

  enable_nat_gateway  = true
  single_nat_gateway  = false
  reuse_nat_ips       = true
  external_nat_ip_ids = aws_eip.nat.*.id
}

resource "aws_service_discovery_private_dns_namespace" "main" {
  name        = "thunder.dome"
  description = "private dns"
  vpc         = module.vpc.vpc_id
}

resource "aws_eip" "nat" {
  count = 3
  vpc   = true
}

resource "aws_eip" "ecs" {
  count = 1
  vpc   = true
}

resource "aws_lb" "ecs" {
  name               = "ecs"
  load_balancer_type = "network"

  subnet_mapping {
    subnet_id     = module.vpc.public_subnets[0]
    allocation_id = aws_eip.ecs[0].id
  }
}

resource "aws_ecr_repository" "thunderdome" {
  name                 = "thunderdome"
  image_tag_mutability = "MUTABLE"
}

resource "aws_ecr_repository" "grafana-agent" {
  name                 = "grafana-agent"
  image_tag_mutability = "MUTABLE"
}

resource "aws_ecr_repository" "dealgood" {
  name                 = "dealgood"
  image_tag_mutability = "MUTABLE"
}

resource "aws_ecr_repository" "skyfish" {
  name                 = "skyfish"
  image_tag_mutability = "MUTABLE"
}

resource "aws_ecr_repository" "ironbar" {
  name                 = "ironbar"
  image_tag_mutability = "MUTABLE"
}

resource "aws_cloudwatch_log_group" "logs" {
  name = "thunderdome"
}

