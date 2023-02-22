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

variable "dealgood_tag" {
  default = "2022-09-15__1504"
}

# variable "ssm_exec_policy_arn" {
# }

variable "target_task_role_arn" {
}

variable "dealgood_task_role_arn" {
}

variable "dealgood_secrets" {
}

variable "dealgood_environment" {
  type = list(map(string))
  default = []
}

variable "efs_file_system_id" {
}

variable "grafana_agent_dealgood_config_url" {
}

variable "grafana_agent_target_config_url" {
}

variable "request_rate" {
}

variable "request_filter" {
  type    = string
  default = "pathonly"
}

variable "request_source" {
  type    = string
}

variable "request_sns_topic_arn" {
  description = "arn of an sns topic to subscribe to"
}

variable "capacity_provider" {
  type    = string
}

variable "target_memory" {
  type        = number
  description = "Amount (in GiB) of memory to allocate to the targets."
}

