locals {
  ecs_cluster_name = "thunderdome"
  user_data        = <<-EOT
    #!/bin/bash
    cat <<'EOF' >> /etc/ecs/ecs.config
    ECS_CLUSTER=${local.ecs_cluster_name}
    ECS_LOGLEVEL=debug
    EOF
    mkfs.xfs /dev/nvme1n1
    mount /dev/nvme1n1 /var/lib/docker/volumes
    cat /proc/mounts | grep nvme1n1 >> /etc/fstab
    systemctl enable fstrim.timer
    systemctl start fstrim.timer
  EOT
}

module "ecs-asg" {
  source = "terraform-aws-modules/ecs/aws"

  cluster_name = local.ecs_cluster_name

  cluster_configuration = {
    execute_command_configuration = {
      logging = "OVERRIDE"
      log_configuration = {
        cloud_watch_log_group_name = aws_cloudwatch_log_group.this.name
      }
    }
  }

  default_capacity_provider_use_fargate = false

  # Capacity provider - Fargate
  fargate_capacity_providers = {
    FARGATE      = {}
    FARGATE_SPOT = {}
  }

  # Capacity provider - autoscaling groups
  autoscaling_capacity_providers = {
    one = {
      auto_scaling_group_arn         = module.autoscaling["one"].autoscaling_group_arn
      managed_termination_protection = "DISABLED"

      managed_scaling = {
        maximum_scaling_step_size = 10
        minimum_scaling_step_size = 1
        status                    = "ENABLED"
        target_capacity           = 100
        # this is a percentage you want to be utilised, hopefully 100% leads to less wasted resources
        # https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ecs_capacity_provider#target_capacity
      }

      default_capacity_provider_strategy = {
        weight = 60
        base   = 20
      }
    }
  }
}

data "aws_ssm_parameter" "ecs_optimized_ami" {
  name = "/aws/service/ecs/optimized-ami/amazon-linux-2/recommended"
}


module "autoscaling" {
  source  = "terraform-aws-modules/autoscaling/aws"
  version = "~> 6.5"

  for_each = {
    one = {
      instance_type = "c6id.8xlarge"
    }
  }

  name = "${local.ecs_cluster_name}-${each.key}"

  image_id      = jsondecode(data.aws_ssm_parameter.ecs_optimized_ami.value)["image_id"]
  instance_type = each.value.instance_type
  key_name      = "thunderdome"

  security_groups = [
    aws_security_group.target.id,
    aws_security_group.allow_ssh.id
  ]

  block_device_mappings = [
    {
      # Root volume
      device_name = "/dev/xvda" # This is /dev/nvme1n1 on the machine
      no_device   = 0
      ebs = {
        delete_on_termination = true
        encrypted             = false
        volume_size           = 200
        volume_type           = "gp3"
        iops                  = 200 * 50 # You can have 50xVolumeSize
        throughput            = 1000     # 1000 is max for gp3
      }
    }
  ]

  user_data                       = base64encode(local.user_data)
  ignore_desired_capacity_changes = true

  create_iam_instance_profile = true
  iam_role_name               = local.ecs_cluster_name
  iam_role_description        = "ECS role for ${local.ecs_cluster_name}"
  iam_role_policies = {
    AmazonEC2ContainerServiceforEC2Role = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"
    AmazonSSMManagedInstanceCore        = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
  }

  network_interfaces = [
    {
      delete_on_termination       = true
      associate_public_ip_address = true
      description                 = "eth0"
      device_index                = 0
      # security_groups             = []
    }
  ]

  instance_refresh = {
    strategy = "Rolling"
    preferences = {
      min_healthy_percentage = 0
    }
    triggers = ["tag"]
  }

  vpc_zone_identifier = module.vpc.public_subnets
  health_check_type   = "EC2"
  min_size            = 0
  max_size            = 100
  desired_capacity    = 1

  # https://github.com/hashicorp/terraform-provider-aws/issues/12582
  autoscaling_group_tags = {
    AmazonECSManaged = true
  }

  protect_from_scale_in = false
}

resource "aws_cloudwatch_log_group" "this" {
  name              = "/aws/ecs/thunderdome-asg"
  retention_in_days = 7
}
