variable "name" {
  type        = string
  description = ""
}

variable "ecs_cluster_id" {
  type = string
}

variable "vpc_subnets" {
  type = list(string)
}

variable "security_groups" {
  type = list(string)
}

variable "execution_role_arn" {
  type = string
}

variable "shared_env" {
  type = list(map(string))
}

variable "targets" {}

variable "log_group_name" {}

variable "aws_service_discovery_private_dns_namespace_id" {}

variable "grafana_secrets" {
  default = []
}

variable "grafana_agent_tag" {
  default = "150e302"
}
