resource "aws_ecs_service" "skyfish" {
  name                   = "skyfish"
  cluster                = module.ecs-asg.cluster_id
  task_definition        = aws_ecs_task_definition.skyfish.arn
  desired_count          = 1
  enable_execute_command = true

  network_configuration {
    subnets          = module.vpc.public_subnets
    security_groups  = [aws_security_group.skyfish.id]
    assign_public_ip = true
  }

  capacity_provider_strategy {
    base              = 0
    capacity_provider = "FARGATE"
    weight            = 1
  }
}

resource "aws_ecs_task_definition" "skyfish" {
  family                   = "skyfish-something-else"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecsTaskExecutionRole.arn
  task_role_arn            = aws_iam_role.skyfish.arn

  cpu    = 4 * 1024
  memory = 10 * 1024

  ephemeral_storage {
    size_in_gib = 200
  }

  volume {
    name = "grafana-agent-data"
  }

  volume {
    name = "efs"
    efs_volume_configuration {
      file_system_id = aws_efs_file_system.thunderdome.id
    }
  }

  container_definitions = jsonencode([
    {
      name      = "skyfish"
      image     = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/skyfish:${local.skyfish_image_tag}"
      cpu       = 0
      essential = true

      environment = [
        { name = "OTEL_TRACES_EXPORTER", value = "otlp" },
        { name = "OTEL_EXPORTER_OTLP_ENDPOINT", value = "http://localhost:4317" },
        { name = "SKYFISH_LOKI_URI", value = "https://logs-prod-us-central1.grafana.net" },
        { name = "SKYFISH_LOKI_QUERY", value = "{job=\"nginx\",app=\"gateway\",team=\"bifrost\"}" },
        { name = "SKYFISH_TOPIC", value = "${aws_sns_topic.gateway_requests.arn}" },
        { name = "SKYFISH_SNS_REGION", value = "${data.aws_region.current.name}" },
        { name = "SKYFISH_PROMETHEUS_ADDR", value = ":9090" },
      ]

      secrets = [
        { name = "SKYFISH_LOKI_USERNAME", valueFrom = "${data.aws_secretsmanager_secret.dealgood-loki-secret.arn}:username::" },
        { name = "SKYFISH_LOKI_PASSWORD", valueFrom = "${data.aws_secretsmanager_secret.dealgood-loki-secret.arn}:password::" },
      ]


      logConfiguration = {
        logDriver = "awslogs",
        options = {
          awslogs-group         = aws_cloudwatch_log_group.logs.name,
          awslogs-region        = "${data.aws_region.current.name}",
          awslogs-stream-prefix = "ecs"
        }
      }

      mountPoints = [{
        sourceVolume  = "efs"
        containerPath = "/mnt/efs"
      }]

      portMappings = [
        { containerPort = 8080, hostPort = 8080, protocol = "tcp" },
      ]

      ulimits = [
        {
          name      = "nofile",
          hardLimit = 1048576,
          softLimit = 1048576
        }
      ]
      volumesFrom = []
    },
    {
      name  = "grafana-agent"
      cpu   = 0
      image = "grafana/agent:v0.39.1"
      command = [
        "-metrics.wal-directory=/data/grafana-agent",
        "-config.expand-env",
        "-enable-features=remote-configs",
        "-config.file=https://${module.s3_bucket_public.s3_bucket_bucket_domain_name}/${module.grafana_agent_config["skyfish"].s3_object_id}"
      ]
      environment = [
      ]
      essential = true
      logConfiguration = {
        logDriver = "awslogs",
        options = {
          awslogs-group         = aws_cloudwatch_log_group.logs.name,
          awslogs-region        = "${data.aws_region.current.name}",
          awslogs-stream-prefix = "ecs"
        }
      }
      mountPoints = [
        {
          sourceVolume  = "grafana-agent-data",
          containerPath = "/data/grafana-agent"
        },
        {
          sourceVolume  = "efs"
          containerPath = "/mnt/efs"
        }
      ]
      portMappings = []
      secrets = [
        { name = "PROMETHEUS_URL", valueFrom = "${data.aws_secretsmanager_secret.prometheus-secret.arn}:url::" },
        { name = "PROMETHEUS_USER", valueFrom = "${data.aws_secretsmanager_secret.prometheus-secret.arn}:username::" },
        { name = "PROMETHEUS_PASS", valueFrom = "${data.aws_secretsmanager_secret.prometheus-secret.arn}:password::" }
      ]
      volumesFrom = []
    }
  ])
}

