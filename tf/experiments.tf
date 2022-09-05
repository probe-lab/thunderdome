module "tweedles" {
  source = "./modules/experiment"
  name   = "tweedles"

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
    "dee" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0"
      environment = []
    },
    "dum" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0"
      environment = []
    }
  }
}

module "reposize" {
  source = "./modules/experiment"
  name   = "reposize"

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
    "005GB" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0-reposize"
      environment = [
        { name = "REPO_SIZE", value = "5GB" }
      ]
    }

    "010GB" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0-reposize"
      environment = [
        { name = "REPO_SIZE", value = "10GB" }
      ]
    }

    "025GB" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0-reposize"
      environment = [
        { name = "REPO_SIZE", value = "25GB" }
      ]
    }

    "050GB" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0-reposize"
      environment = [
        { name = "REPO_SIZE", value = "50GB" }
      ]
    }

    "075GB" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0-reposize"
      environment = [
        { name = "REPO_SIZE", value = "75GB" }
      ]
    }

    "100GB" = {
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0-reposize"
      environment = [
        { name = "REPO_SIZE", value = "100GB" }
      ]
    },
  }
}

module "releases" {
  source = "./modules/experiment"
  name   = "releases"

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
    "v0_08_0" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.8.0"
      environment = []
    },
    "v0_09_1" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.9.1"
      environment = []
    },
    "v0_10_0" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.10.0"
      environment = []
    },
    "v0_11_0" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.11.0"
      environment = []
    },
    "v0_12_2" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.12.2"
      environment = []
    },
    "v0_13_1" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.13.1"
      environment = []
    },
    "v0_14_0" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.14.0"
      environment = []
    },
    "v0_15_0" = {
      image       = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0"
      environment = []
    }
  }
}
