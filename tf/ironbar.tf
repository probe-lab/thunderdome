resource "aws_ecs_service" "ironbar" {
  name                   = "ironbar"
  cluster                = module.ecs-asg.cluster_id
  task_definition        = aws_ecs_task_definition.ironbar.arn
  desired_count          = 1
  enable_execute_command = true

  network_configuration {
    subnets          = module.vpc.public_subnets
    security_groups  = [aws_security_group.ironbar.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.ironbar.id
    container_name   = "ironbar"
    container_port   = local.ironbar_port_number
  }

  capacity_provider_strategy {
    base              = 0
    capacity_provider = "FARGATE"
    weight            = 1
  }
}

resource "aws_service_discovery_service" "ironbar" {
  name = "ironbar"

  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.main.id

    dns_records {
      ttl  = 10
      type = "SRV"
    }

    routing_policy = "MULTIVALUE"
  }
}

resource "aws_lb_target_group" "ironbar" {
  name        = "ironbar"
  port        = local.ironbar_port_number
  protocol    = "TCP"
  vpc_id      = module.vpc.vpc_id
  target_type = "ip"
}

resource "aws_lb_listener" "front_end" {
  load_balancer_arn = aws_lb.ecs.id
  port              = local.ironbar_port_number
  protocol          = "TCP"
  default_action {
    target_group_arn = aws_lb_target_group.ironbar.id
    type             = "forward"
  }
}

resource "aws_ecs_task_definition" "ironbar" {
  family                   = "ironbar"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecsTaskExecutionRole.arn
  task_role_arn            = aws_iam_role.ironbar.arn

  cpu    = 2 * 1024
  memory = 4 * 1024

  ephemeral_storage {
    size_in_gib = 30
  }

  volume {
    name = "grafana-agent-data"
  }

  container_definitions = jsonencode([
    {
      name      = "ironbar"
      image     = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/ironbar:${local.ironbar_image_tag}"
      cpu       = 0
      essential = true

      environment = [
        { name = "OTEL_TRACES_EXPORTER", value = "otlp" },
        { name = "OTEL_EXPORTER_OTLP_ENDPOINT", value = "http://localhost:4317" },
        { name = "IRONBAR_ADDR", value = ":${local.ironbar_port_number}" },
        { name = "IRONBAR_DIAG_ADDR", value = ":9090" },
        { name = "IRONBAR_VERY_VERBOSE", value = "true" },
        { name = "IRONBAR_EXPERIMENTS_TABLE_NAME", value = "${aws_dynamodb_table.experiments.name}" },
        { name = "IRONBAR_MONITOR_INTERVAL", value = "1" },
        { name = "IRONBAR_SETTLE", value = "5" },
      ]

      logConfiguration = {
        logDriver = "awslogs",
        options = {
          awslogs-group         = aws_cloudwatch_log_group.logs.name,
          awslogs-region        = "${data.aws_region.current.name}",
          awslogs-stream-prefix = "ecs"
        }
      }

      portMappings = [
        { containerPort = local.ironbar_port_number, hostPort = local.ironbar_port_number, protocol = "tcp" },
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
        "-config.file=https://${module.s3_bucket_public.s3_bucket_bucket_domain_name}/${module.grafana_agent_config["ironbar"].s3_object_id}"
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

