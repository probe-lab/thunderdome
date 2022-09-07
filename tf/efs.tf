resource "aws_efs_file_system" "thunderdome" {
  creation_token         = "thunderdome"
  availability_zone_name = module.vpc.azs[0]
  lifecycle_policy {
    transition_to_ia = "AFTER_7_DAYS"
  }
}

resource "aws_efs_mount_target" "thunderdome" {
  file_system_id  = aws_efs_file_system.thunderdome.id
  subnet_id       = module.vpc.public_subnets[0]
  security_groups = [aws_security_group.efs.id]
}

resource "aws_security_group" "efs" {
  name   = "efs"
  vpc_id = module.vpc.vpc_id
}

resource "aws_security_group_rule" "efs-allow" {
  security_group_id        = aws_security_group.efs.id
  type                     = "ingress"
  from_port                = 2049
  to_port                  = 2049
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.use_efs.id
}

resource "aws_security_group" "use_efs" {
  name   = "use_efs"
  vpc_id = module.vpc.vpc_id
}
