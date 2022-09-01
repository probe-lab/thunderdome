module "tracing" {
  source = "./modules/experiment"
  name   = "tracing"

  ecs_cluster_id                                 = module.ecs-asg.cluster_id
  vpc_subnets                                    = module.vpc.public_subnets
  target_security_groups                         = [aws_security_group.target.id]
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
    { name = "OTEL_TRACES_SAMPLER", value = "traceidratio" },
    { name = "OTEL_TRACES_EXPORTER", value = "otlp" },
    { name = "OTEL_EXPORTER_OTLP_INSECURE", value = "true" },
    { name = "OTEL_EXPORTER_OTLP_ENDPOINT", value = "http://localhost:4318" }
  ]

  targets = {
    "0" = {
      environment = [
        { name = "OTEL_TRACES_SAMPLER_ARG", value = "0" }
      ],
      image = "ipfs/kubo:v0.14.0"
    },
    # "25" = {
    #   environment = [
    #     { name = "OTEL_TRACES_SAMPLER_ARG", value = "0.25" }
    #   ]
    #   image = "ipfs/kubo:v0.14.0"
    # }
    # "50" = {
    #   environment = [
    #     { name = "OTEL_TRACES_SAMPLER_ARG", value = "0.5" }
    #   ]
    #   image = "ipfs/kubo:v0.14.0"
    # }
    # "75" = {
    #   environment = [
    #     { name = "OTEL_TRACES_SAMPLER_ARG", value = "0.75" }
    #   ]
    #   image = "ipfs/kubo:v0.14.0"
    # }
    "100" = {
      environment = [
        { name = "OTEL_TRACES_SAMPLER_ARG", value = "1" }
      ]
      image = "ipfs/kubo:v0.14.0"
    }
  }
}

module "peering" {
  source = "./modules/experiment"
  name   = "peering"

  ecs_cluster_id                                 = module.ecs-asg.cluster_id
  vpc_subnets                                    = module.vpc.public_subnets
  target_security_groups                         = [aws_security_group.target.id]
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
    "with" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:peering-with"
      environment = []
    }
    "without" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:peering-without"
      environment = []
    }
  }
}

