module "provdelayrouting1000" {
  count          = 0 # 0 is disabled, 1 is enabled
  source         = "./modules/experiment"
  name           = "provdelayrouting1000"
  request_rate   = 10
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
  capacity_provider                              = "io_medium"
  target_memory                                  = 30

  grafana_secrets = [
    { name = "GRAFANA_USER", valueFrom = "${data.aws_secretsmanager_secret.grafana-push-secret.arn}:username::" },
    { name = "GRAFANA_PASS", valueFrom = "${data.aws_secretsmanager_secret.grafana-push-secret.arn}:password::" }
  ]

  dealgood_secrets = [
    { name = "DEALGOOD_LOKI_USERNAME", valueFrom = "${data.aws_secretsmanager_secret.dealgood-loki-secret.arn}:username::" },
    { name = "DEALGOOD_LOKI_PASSWORD", valueFrom = "${data.aws_secretsmanager_secret.dealgood-loki-secret.arn}:password::" },
  ]

  shared_env = [
    { name = "IPFS_PROFILE", value = "server" },
  ]

  targets = {
    "kubo17-dht-900" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-0.17.0-provdelayrouting-r2"
      environment = [
        { name = "ROUTING_TYPE", value = "dht" },
        { name = "CONNMGR_HIGHWATER", value = "900" },
        { name = "CONNMGR_LOWWATER", value = "600" },
        { name = "BITSWAP_PROVIDERSEARCHDELAY", value = "1000ms" },
      ]
    }
    "kubo18rc2-auto-96" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-0.18.0-rc2-provdelayrouting-r2"
      environment = [
        { name = "ROUTING_TYPE", value = "auto" },
        { name = "CONNMGR_HIGHWATER", value = "96" },
        { name = "CONNMGR_LOWWATER", value = "32" },
        { name = "BITSWAP_PROVIDERSEARCHDELAY", value = "1000ms" },
      ]
    }
    "kubo18rc2-dht-96" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-0.18.0-rc2-provdelayrouting-r2"
      environment = [
        { name = "ROUTING_TYPE", value = "dht" },
        { name = "CONNMGR_HIGHWATER", value = "96" },
        { name = "CONNMGR_LOWWATER", value = "32" },
        { name = "BITSWAP_PROVIDERSEARCHDELAY", value = "1000ms" },
      ]
    }
    "kubo18rc2-dht-900" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-0.18.0-rc2-provdelayrouting-r2"
      environment = [
        { name = "ROUTING_TYPE", value = "dht" },
        { name = "CONNMGR_HIGHWATER", value = "900" },
        { name = "CONNMGR_LOWWATER", value = "600" },
        { name = "BITSWAP_PROVIDERSEARCHDELAY", value = "1000ms" },
      ]
    }
  }
}

