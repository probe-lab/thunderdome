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

resource "aws_security_group" "dealgood" {
  name   = "dealgood"
  vpc_id = module.vpc.vpc_id
  egress {
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }
}

resource "aws_security_group" "allow_ssh" {
  name   = "allow_ssh"
  vpc_id = module.vpc.vpc_id

  ingress {
    from_port        = 22
    to_port          = 22
    protocol         = "tcp"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }
}

resource "aws_security_group" "allow_ipfs" {
  name   = "allow_ipfs"
  vpc_id = module.vpc.vpc_id
}

resource "aws_security_group_rule" "allow_ipfs_egress" {
  security_group_id = aws_security_group.allow_ipfs.id
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  ipv6_cidr_blocks  = ["::/0"]
}

resource "aws_security_group_rule" "allow_ipfs" {
  security_group_id = aws_security_group.allow_ipfs.id
  type              = "ingress"
  from_port         = 4001
  to_port           = 4001
  protocol          = "tcp"
  cidr_blocks       = ["0.0.0.0/0"]
  ipv6_cidr_blocks  = ["::/0"]
}

resource "aws_security_group_rule" "allow_ipfs_udp" {
  security_group_id = aws_security_group.allow_ipfs.id
  type              = "ingress"
  from_port         = 4001
  to_port           = 4001
  protocol          = "udp"
  cidr_blocks       = ["0.0.0.0/0"]
  ipv6_cidr_blocks  = ["::/0"]
}
