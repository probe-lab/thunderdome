module "tracing" {
  source = "./modules/experiment"

  name = "tracing"

  ecs_cluster_id     = module.ecs.cluster_id
  vpc_subnets        = module.vpc.public_subnets
  security_groups    = [aws_security_group.target.id]
  execution_role_arn = aws_iam_role.ecsTaskExecutionRole.arn
  log_group_name     = aws_cloudwatch_log_group.logs.name

  aws_service_discovery_private_dns_namespace_id = aws_service_discovery_private_dns_namespace.main.id

  shared_env = [
    { name = "IPFS_PROFILE", value = "server" },
    { name = "OTEL_TRACES_SAMPLER", value = "traceidratio" },
    { name = "OTEL_TRACES_EXPORTER", value = "file" },
    { name = "OTEL_EXPORTER_FILE_PATH", value = "/dev/null" }
  ]

  targets = {
    "0" = {
      environment = [
        { name = "OTEL_TRACES_SAMPLER_ARG", value = "0" }
      ],
      image = "ipfs/kubo:v0.14.0"
    },
    "25" = {
      environment = [
        { name = "OTEL_TRACES_SAMPLER_ARG", value = "0.25" }
      ]
      image = "ipfs/kubo:v0.14.0"
    }
    "50" = {
      environment = [
        { name = "OTEL_TRACES_SAMPLER_ARG", value = "0.5" }
      ]
      image = "ipfs/kubo:v0.14.0"
    }
    "75" = {
      environment = [
        { name = "OTEL_TRACES_SAMPLER_ARG", value = "0.75" }
      ]
      image = "ipfs/kubo:v0.14.0"
    }
    "100" = {
      environment = [
        { name = "OTEL_TRACES_SAMPLER_ARG", value = "1" }
      ]
      image = "ipfs/kubo:v0.14.0"
    }
  }
}

resource "aws_security_group" "target" {
  name   = "target"
  vpc_id = module.vpc.vpc_id
}

resource "aws_security_group_rule" "target_allow_egress" {
  security_group_id = aws_security_group.target.id
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  ipv6_cidr_blocks  = ["::/0"]
}

resource "aws_security_group_rule" "target_allow_ipfs" {
  security_group_id = aws_security_group.target.id
  type              = "ingress"
  from_port         = 4001
  to_port           = 4001
  protocol          = "tcp"
  cidr_blocks       = ["0.0.0.0/0"]
  ipv6_cidr_blocks  = ["::/0"]
}

resource "aws_security_group_rule" "target_allow_ipfs_udp" {
  security_group_id = aws_security_group.target.id
  type              = "ingress"
  from_port         = 4001
  to_port           = 4001
  protocol          = "udp"
  cidr_blocks       = ["0.0.0.0/0"]
  ipv6_cidr_blocks  = ["::/0"]
}

resource "aws_security_group_rule" "target_allow_gateway" {
  security_group_id        = aws_security_group.target.id
  type                     = "ingress"
  from_port                = 8080
  to_port                  = 8080
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.dealgood.id
}
