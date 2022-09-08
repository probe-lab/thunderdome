module "providerdelay" {
  source = "./modules/experiment"
  name   = "providerdelay"

  ecs_cluster_id                                 = module.ecs-asg.cluster_id
  efs_file_system_id                             = aws_efs_file_system.thunderdome.id
  vpc_subnets                                    = module.vpc.public_subnets
  dealgood_security_groups                       = [aws_security_group.dealgood.id]
  execution_role_arn                             = aws_iam_role.ecsTaskExecutionRole.arn
  dealgood_task_role_arn                         = aws_iam_role.dealgood.arn
  log_group_name                                 = aws_cloudwatch_log_group.logs.name
  aws_service_discovery_private_dns_namespace_id = aws_service_discovery_private_dns_namespace.main.id
  ssm_exec_policy_arn                            = aws_iam_policy.ssm-exec.arn

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
    "0ms" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-providerdelay-from-env"
      environment = [
        { name = "SEARCH_DELAY", value = "0" }
      ]
    }
    "20ms" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-providerdelay-from-env"
      environment = [
        { name = "SEARCH_DELAY", value = "20ms" }
      ]
    }
    "50ms" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-providerdelay-from-env"
      environment = [
        { name = "SEARCH_DELAY", value = "50ms" }
      ]
    }
    "100ms" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-providerdelay-from-env"
      environment = [
        { name = "SEARCH_DELAY", value = "100ms" }
      ]
    }
    "200ms" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-providerdelay-from-env"
      environment = [
        { name = "SEARCH_DELAY", value = "200ms" }
      ]
    }
    "500ms" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-providerdelay-from-env"
      environment = [
        { name = "SEARCH_DELAY", value = "500ms" }
      ]
    }
    "750ms" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-providerdelay-from-env"
      environment = [
        { name = "SEARCH_DELAY", value = "750ms" }
      ]
    }
    "1000ms" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-providerdelay-from-env"
      environment = [
        { name = "SEARCH_DELAY", value = "1000ms" }
      ]
    }
  }
}
