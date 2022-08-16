data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

resource "aws_ecs_service" "target" {
  for_each               = var.targets
  name                   = "${var.name}-${each.key}"
  cluster                = var.ecs_cluster_id
  task_definition        = aws_ecs_task_definition.target[each.key].arn
  desired_count          = 1
  enable_execute_command = true

  service_registries {
    registry_arn = aws_service_discovery_service.target[each.key].arn
  }

  network_configuration {
    subnets          = var.vpc_subnets
    security_groups  = var.security_groups
    assign_public_ip = true
  }

  capacity_provider_strategy {
    base              = 0
    capacity_provider = "FARGATE"
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
      type = "A"
    }

    routing_policy = "MULTIVALUE"
  }
}

# note: to keep terraform from recreating this every time, keep the container definition JSON alphabetized
resource "aws_ecs_task_definition" "target" {
  for_each                 = var.targets
  family                   = "${var.name}-${each.key}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  execution_role_arn       = var.execution_role_arn
  task_role_arn            = aws_iam_role.experiment.arn

  cpu    = 4 * 1024
  memory = 30 * 1024

  tags     = {}
  tags_all = {}

  ephemeral_storage {
    size_in_gib = 200
  }

  volume {
    name = "ipfs-data"
  }

  volume {
    name = "grafana-agent-data"
  }

  container_definitions = jsonencode([
    {
      name      = "gateway"
      image     = each.value.image
      cpu       = 0
      essential = true

      environment = concat(var.shared_env, each.value.environment)

      mountPoints = [{
        sourceVolume  = "ipfs-data",
        containerPath = "/data/ipfs"
      }]

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
      image = "147263665150.dkr.ecr.eu-west-1.amazonaws.com/grafana-agent:${var.grafana_agent_tag}"
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
      mountPoints = [{
        sourceVolume  = "grafana-agent-data",
        containerPath = "/data/grafana-agent"
      }]
      name         = "grafana-agent"
      portMappings = []
      secrets      = var.grafana_secrets
      volumesFrom  = []
    }
  ])
}

resource "aws_iam_role" "experiment" {
  name = var.name
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      },
    ]
  })
}

resource "aws_iam_role_policy" "experiment" {
  name = var.name
  role = aws_iam_role.experiment.id

  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      # allow SSH access via SSM
      {
        "Effect" : "Allow",
        "Action" : [
          "ssmmessages:CreateControlChannel",
          "ssmmessages:CreateDataChannel",
          "ssmmessages:OpenControlChannel",
          "ssmmessages:OpenDataChannel"
        ],
        "Resource" : "*"
      }
    ]
  })
}
