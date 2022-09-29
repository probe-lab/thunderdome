module "commits" {
  source = "./modules/experiment"
  name   = "commits-2022-09-29"
  request_rate = 20
  request_source = "sqs"

  ecs_cluster_id                                 = module.ecs-asg.cluster_id
  efs_file_system_id                             = aws_efs_file_system.thunderdome.id
  vpc_subnets                                    = module.vpc.public_subnets
  dealgood_security_groups                       = [aws_security_group.dealgood.id]
  execution_role_arn                             = aws_iam_role.ecsTaskExecutionRole.arn
  dealgood_task_role_arn                         = aws_iam_role.dealgood.arn
  log_group_name                                 = aws_cloudwatch_log_group.logs.name
  aws_service_discovery_private_dns_namespace_id = aws_service_discovery_private_dns_namespace.main.id
  ssm_exec_policy_arn                            = aws_iam_policy.ssm-exec.arn
  grafana_agent_dealgood_config_url              = "http://${module.s3_bucket_public.s3_bucket_bucket_domain_name}/${module.grafana_agent_config["dealgood"].s3_object_id}"
  grafana_agent_target_config_url                = "http://${module.s3_bucket_public.s3_bucket_bucket_domain_name}/${module.grafana_agent_config["target"].s3_object_id}"
  request_sns_topic_arn                          = aws_sns_topic.gateway_requests.arn

  grafana_secrets = [
    { name = "GRAFANA_USER", valueFrom = "${data.aws_secretsmanager_secret.grafana-push-secret.arn}:username::" },
    { name = "GRAFANA_PASS", valueFrom = "${data.aws_secretsmanager_secret.grafana-push-secret.arn}:password::" }
  ]

  dealgood_environment = [
    { name = "DEALGOOD_SOURCE", value = "loki" },
    { name = "DEALGOOD_LOKI_URI", value = "https://logs-prod-us-central1.grafana.net" },
    { name = "DEALGOOD_LOKI_QUERY", value = "{job=\"nginx\",app=\"gateway\",team=\"bifrost\"}" },
  ]

  dealgood_secrets = [
    { name = "DEALGOOD_LOKI_USERNAME", valueFrom = "${data.aws_secretsmanager_secret.dealgood-loki-secret.arn}:username::" },
    { name = "DEALGOOD_LOKI_PASSWORD", valueFrom = "${data.aws_secretsmanager_secret.dealgood-loki-secret.arn}:password::" },
  ]

  shared_env = [
    { name = "IPFS_PROFILE", value = "server" },
  ]

  targets = {
    "kubo_d1b9e41fc_a" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:gw-kubo-commit-d1b9e41fc"
      environment = []
    },
    "kubo_d1b9e41fc_b" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:gw-kubo-commit-d1b9e41fc"
      environment = []
    },
    "kubo_5bcbd153f_a" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:gw-kubo-commit-5bcbd153f"
      environment = []
    },
    "kubo_5bcbd153f_b" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:gw-kubo-commit-5bcbd153f"
      environment = []
    },
    "kubo_aeaf57734_a" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:gw-kubo-commit-aeaf57734"
      environment = []
    },
    "kubo_aeaf57734_b" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:gw-kubo-commit-aeaf57734"
      environment = []
    },
    "kubo_196887cbe_a" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:gw-kubo-commit-196887cbe"
      environment = []
    },
    "kubo_196887cbe_b" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:gw-kubo-commit-196887cbe"
      environment = []
    }
  }
}
