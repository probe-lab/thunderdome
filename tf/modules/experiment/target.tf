resource "aws_ecs_service" "target" {
  for_each               = var.targets
  name                   = "${var.name}-${each.key}"
  cluster                = var.ecs_cluster_id
  task_definition        = aws_ecs_task_definition.target[each.key].arn
  desired_count          = 1
  enable_execute_command = true

  service_registries {
    registry_arn   = aws_service_discovery_service.target[each.key].arn
    container_port = 8080
    container_name = "gateway"
  }

  capacity_provider_strategy {
    base              = 0
    capacity_provider = var.capacity_provider
    weight            = 1
  }
}

resource "aws_service_discovery_service" "target" {
  for_each = var.targets
  name     = "${var.name}-${each.key}"

  dns_config {
    namespace_id = var.aws_service_discovery_private_dns_namespace_id

    dns_records {
      ttl  = 10
      type = "SRV"
    }

    routing_policy = "MULTIVALUE"
  }
}

# note: to keep terraform from recreating this every time, keep the container definition JSON alphabetized
resource "aws_ecs_task_definition" "target" {
  for_each                 = var.targets
  family                   = "${var.name}-${each.key}"
  requires_compatibilities = ["EC2"]
  network_mode             = "host"
  execution_role_arn       = var.execution_role_arn
  task_role_arn            = var.target_task_role_arn #aws_iam_role.experiment.arn

  memory = var.target_memory * 1024

  tags = {
    "experiment" = var.name
    "target"     = each.key
  }

  volume {
    name = "ipfs-data"
  }

  volume {
    name = "grafana-agent-data"
  }

  volume {
    name = "ecs-exporter-data"
  }

  volume {
    name = "efs"
    efs_volume_configuration {
      file_system_id = var.efs_file_system_id
    }
  }

  container_definitions = jsonencode([
    {
      name      = "gateway"
      image     = each.value.image
      cpu       = 0
      essential = true

      environment = concat(var.shared_env, each.value.environment)

      mountPoints = [
        {
          sourceVolume  = "ipfs-data",
          containerPath = "/data/ipfs"
        },
        {
          sourceVolume  = "efs"
          containerPath = "/mnt/efs"
        }
      ]

      logConfiguration = {
        logDriver = "awslogs",
        options = {
          awslogs-group         = var.log_group_name,
          awslogs-region        = "${data.aws_region.current.name}",
          awslogs-stream-prefix = "ecs"
        }
      }

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
      cpu   = 0
      image = "grafana/agent:v0.39.1"
      command = [
        "-metrics.wal-directory=/data/grafana-agent",
        "-config.expand-env",
        "-enable-features=remote-configs",
        "-config.file=${var.grafana_agent_target_config_url}"
      ]
      environment = [
        # we use these for setting labels on metrics
        { name = "THUNDERDOME_EXPERIMENT", value = var.name },
        { name = "THUNDERDOME_TARGET", value = each.key }
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
      name         = "grafana-agent"
      portMappings = []
      secrets      = var.grafana_secrets
      volumesFrom  = []
    },
    {
      cpu       = 0
      environment = []
      essential = true
      # v0.2.0 is broken see https://github.com/prometheus-community/ecs_exporter/issues/40
      image     = "quay.io/prometheuscommunity/ecs-exporter:v0.1.1"
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
          sourceVolume  = "ecs-exporter-data",
          containerPath = "/data/ecs-exporter"
        },
        {
          sourceVolume  = "efs"
          containerPath = "/mnt/efs"
        }
      ]
      name      = "ecs-exporter"
      portMappings = [
        { containerPort = 9779, hostPort = 9779, protocol = "tcp" },
      ]
      volumesFrom  = []
    }
  ])
}

# resource "aws_iam_role" "experiment" {
#   name = var.name
#   assume_role_policy = jsonencode({
#     Version = "2012-10-17"
#     Statement = [
#       {
#         Action = "sts:AssumeRole"
#         Effect = "Allow"
#         Sid    = ""
#         Principal = {
#           Service = "ecs-tasks.amazonaws.com"
#         }
#       },
#     ]
#   })
# }

# resource "aws_iam_role_policy_attachment" "experiment-ssm" {
#   role       = aws_iam_role.experiment.name
#   policy_arn = var.ssm_exec_policy_arn
# }
