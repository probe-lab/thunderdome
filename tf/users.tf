variable "admins" {
  type = map(object({
    key_name          = string
    provision_workbox = bool
    instance_type     = string
    ami               = string
  }))
  default = {
    "ian.davis" = {
      key_name          = "ian.davis"
      provision_workbox = true
      instance_type     = "t2.small"
      ami               = "ami-0591c8c8aa7d9b217" # debian 11
    }
    "tom.hall" = {
      key_name          = "tom.hall"
      provision_workbox = false
      instance_type     = "t2.small"
      ami               = "ami-0333305f9719618c7" # ubuntu
    }
  }
}

locals {
  deployers = tolist(["gus.eggert"])
}


resource "aws_iam_user" "admin" {
  for_each = var.admins
  name     = each.key
}

resource "aws_iam_user_policy_attachment" "admin" {
  for_each   = aws_iam_user.admin
  user       = each.value.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}

resource "aws_instance" "testbox" {
  for_each      = { for k, v in var.admins : k => v if v.provision_workbox }
  ami           = each.value["ami"]
  instance_type = each.value["instance_type"]
  key_name      = each.value["key_name"]
  iam_instance_profile = aws_iam_instance_profile.testbox_profile.name
  vpc_security_group_ids = [
    aws_security_group.dealgood.id,
    aws_security_group.allow_ssh.id,
    aws_security_group.allow_ipfs.id,
    aws_security_group.allow_8000.id,
    aws_security_group.efs.id,
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


