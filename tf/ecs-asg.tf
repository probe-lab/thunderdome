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
    compute_large = {
      auto_scaling_group_arn         = module.autoscaling["compute_large"].autoscaling_group_arn
      managed_termination_protection = "DISABLED"

      managed_scaling = {
        maximum_scaling_step_size = 10
        minimum_scaling_step_size = 1
        status                    = "ENABLED"
        target_capacity           = 100
      }
    }
    compute_medium = {
      auto_scaling_group_arn         = module.autoscaling["compute_medium"].autoscaling_group_arn
      managed_termination_protection = "DISABLED"

      managed_scaling = {
        maximum_scaling_step_size = 10
        minimum_scaling_step_size = 1
        status                    = "ENABLED"
        target_capacity           = 100
      }
    }
    compute_small = {
      auto_scaling_group_arn         = module.autoscaling["compute_small"].autoscaling_group_arn
      managed_termination_protection = "DISABLED"

      managed_scaling = {
        maximum_scaling_step_size = 10
        minimum_scaling_step_size = 1
        status                    = "ENABLED"
        target_capacity           = 100
      }
    }
    io_large = {
      auto_scaling_group_arn         = module.autoscaling["io_large"].autoscaling_group_arn
      managed_termination_protection = "DISABLED"

      managed_scaling = {
        maximum_scaling_step_size = 10
        minimum_scaling_step_size = 1
        status                    = "ENABLED"
        target_capacity           = 100
      }
    }
    io_medium = {
      auto_scaling_group_arn         = module.autoscaling["io_medium"].autoscaling_group_arn
      managed_termination_protection = "DISABLED"

      managed_scaling = {
        maximum_scaling_step_size = 10
        minimum_scaling_step_size = 1
        status                    = "ENABLED"
        target_capacity           = 100
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
    compute_large = {
      # 64GB RAM, 32 CPU, 12.5 Gigabit, $1.61 hourly
      instance_type = "c6id.8xlarge"
    }

    compute_medium = {
      # 32GB RAM, 16 CPU, Up to 12.5 Gigabit, $0.81 hourly
      instance_type = "c6id.4xlarge"
    }

    compute_small = {
      # 16GB RAM, 8 CPU, Up to 12.5 Gigabit, $0.40 hourly
      instance_type = "c6id.2xlarge"
    }

    io_large = {
      # 64GB RAM, 8 CPU, Up to 25 Gigabit, $0.62 hourly
      instance_type = "i3en.2xlarge"
    }

    io_medium = {
      # 32GB RAM, 4 CPU, Up to 25 Gigabit, $0.31 hourly
      instance_type = "i3en.xlarge"
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
  desired_capacity    = 0

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


