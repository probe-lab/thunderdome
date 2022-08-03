locals {
  admins = toset([
    "ian.davis",
    "tom.hall"
  ])
}

resource "aws_iam_user" "admin" {
  for_each = local.admins
  name     = each.key
}

resource "aws_iam_user_policy_attachment" "admin" {
  for_each   = aws_iam_user.admin
  user       = each.value.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}

resource "aws_instance" "testbox" {
  for_each      = local.admins
  ami           = "ami-030802ad6e5ffb009"
  instance_type = "m6i.xlarge"
  key_name      = each.key
  vpc_security_group_ids = [
    aws_security_group.dealgood.id,
    aws_security_group.allow_ssh.id
  ]
  subnet_id = module.vpc.public_subnets[0]
  tags = {
    Name = each.key
  }
}

resource "aws_eip" "testbox" {
  for_each = aws_instance.testbox
  instance = each.value.id
  vpc      = true
}
