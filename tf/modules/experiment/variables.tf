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
variable "dealgood_security_groups" {
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

variable "target_agent_tag" {
  default = "target-75858d9"
}

variable "dealgood_agent_tag" {
  default = "dealgood-d881af7"
}

variable "dealgood_tag" {
  default = "2022-09-06__1237"
}

variable "ssm_exec_policy_arn" {
}

variable "dealgood_task_role_arn" {
}

variable "dealgood_secrets" {
}
