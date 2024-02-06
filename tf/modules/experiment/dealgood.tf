resource "aws_ecs_service" "dealgood" {
  name                   = "${var.name}-dealgood"
  cluster                = var.ecs_cluster_id
  task_definition        = aws_ecs_task_definition.dealgood.arn
  desired_count          = 1
  enable_execute_command = true

  network_configuration {
    subnets          = var.vpc_subnets
    security_groups  = var.dealgood_security_groups
    assign_public_ip = true
  }

  capacity_provider_strategy {
    base              = 0
    capacity_provider = "FARGATE"
    weight            = 1
  }
}

resource "aws_ecs_task_definition" "dealgood" {
  family                   = "${var.name}-dealgood"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  execution_role_arn       = var.execution_role_arn
  task_role_arn            = var.dealgood_task_role_arn

  cpu    = 4 * 1024
  memory = 10 * 1024

  tags = {
    "experiment" = var.name
  }

  ephemeral_storage {
    size_in_gib = 200
  }

  volume {
    name = "grafana-agent-data"
  }

  volume {
    name = "efs"
    efs_volume_configuration {
      file_system_id = var.efs_file_system_id
    }
  }

  container_definitions = jsonencode([
    {
      name      = "dealgood"
      image     = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/dealgood:${var.dealgood_tag}"
      cpu       = 0
      essential = true

      environment = concat([
        { name = "DEALGOOD_EXPERIMENT", value = var.name },
        { name = "OTEL_TRACES_EXPORTER", value = "otlp" },
        { name = "OTEL_EXPORTER_OTLP_ENDPOINT", value = "http://localhost:4317" },
        { name = "DEALGOOD_TARGETS", value = join(",", [for key, v in var.targets : "${key}::http://${var.name}-${key}.thunder.dome"]) },
        { name = "DEALGOOD_RATE", value = tostring(var.request_rate) },
        { name = "DEALGOOD_FILTER", value = var.request_filter },
        { name = "DEALGOOD_CONCURRENCY", value = "100" },
        { name = "DEALGOOD_DURATION", value = "-1" },
        { name = "DEALGOOD_HOST", value = "ipfs.io" },
        { name = "DEALGOOD_PROMETHEUS_ADDR", value = ":9090" },
        { name = "DEALGOOD_SOURCE", value = var.request_source },
        { name = "DEALGOOD_LOKI_URI", value = "https://logs-prod-us-central1.grafana.net" },
        { name = "DEALGOOD_LOKI_QUERY", value = "{job=\"nginx\",app=\"gateway\",team=\"bifrost\"}" },
        { name = "DEALGOOD_SQS_REGION", value = "${data.aws_region.current.name}" },
        { name = "DEALGOOD_SQS_QUEUE", value = "${aws_sqs_queue.requests.name}" },
      ], var.dealgood_environment)

      secrets = var.dealgood_secrets


      logConfiguration = {
        logDriver = "awslogs",
        options = {
          awslogs-group         = var.log_group_name,
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
        "-config.file=${var.grafana_agent_dealgood_config_url}"
      ]
      environment = [
        # we use these for setting labels on metrics
        { name = "THUNDERDOME_EXPERIMENT", value = var.name },
      ]
      essential = true
      logConfiguration = {
        logDriver = "awslogs",
        options = {
          awslogs-group         = var.log_group_name,
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
      secrets      = var.grafana_secrets
      volumesFrom  = []
    }
  ])
}

