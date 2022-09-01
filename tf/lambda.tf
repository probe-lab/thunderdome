resource "aws_iam_role" "update_nacl" {
  name = "update_nacl"

  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": "sts:AssumeRole",
      "Principal": {
        "Service": "lambda.amazonaws.com"
      },
      "Effect": "Allow",
      "Sid": ""
    }
  ]
}
EOF
}

data "aws_iam_policy_document" "edit_nacl" {
  statement {
    actions = [
      "ec2:DescribeNetworkAcls",
      "ec2:DescribeNetworkInterfaces",
      "ec2:DescribeNetworkInterfaceAttribute",
      "ec2:DescribeInstances",
      "ecs:ListTasks",
      "ecs:DescribeTasks",
      "autoscaling:DescribeAutoScalingGroups"
    ]
    resources = ["*"]
  }

  statement {
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents"
    ]
    resources = [
      "*"
    ]
  }

  statement {
    actions = [
      "ec2:DeleteNetworkAclEntry",
      "ec2:CreateNetworkAclEntry",
      "ec2:ReplaceNetworkAclEntry",
    ]

    resources = [
      "arn:aws:ec2:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:network-acl/${module.vpc.default_network_acl_id}"
    ]
  }
}

resource "aws_cloudwatch_log_group" "update_nacl" {
  name              = "/aws/lambda/${aws_lambda_function.update_nacl.function_name}"
  retention_in_days = 7
  lifecycle {
    prevent_destroy = false
  }
}

resource "aws_iam_policy" "edit_nacl" {
  name   = "edit_nacl"
  path   = "/"
  policy = data.aws_iam_policy_document.edit_nacl.json
}

resource "aws_iam_role_policy_attachment" "edit_nacl" {
  role       = aws_iam_role.update_nacl.name
  policy_arn = aws_iam_policy.edit_nacl.arn
}


resource "aws_lambda_function" "update_nacl" {
  function_name = "update_nacl"
  role          = aws_iam_role.update_nacl.arn
  handler       = "update_nacl.lambda_handler"

  s3_bucket         = aws_s3_bucket.lambdas.id
  s3_key            = aws_s3_object.update_nacl.id
  s3_object_version = aws_s3_object.update_nacl.version_id

  runtime = "python3.9"

  environment {
    variables = {
      NACL_ID   = module.vpc.default_network_acl_id,
      ASG_NAMES = join(",", [for asg in module.autoscaling : asg.autoscaling_group_name])
    }
  }
}

resource "aws_lambda_permission" "with_sns" {
  statement_id  = "AllowExecutionFromCron"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.update_nacl.function_name
  principal     = "events.amazonaws.com"
  source_arn    = module.eventbridge.eventbridge_rule_arns["crons"]
}

data "archive_file" "update_nacl" {
  type        = "zip"
  output_path = "/tmp/update_nacl.zip"
  source {
    content  = file("scripts/update_nacl.py")
    filename = "update_nacl.py"
  }
}

resource "aws_s3_bucket" "lambdas" {
  bucket        = "pl-thunderdome-lambdas"
  force_destroy = true
}

resource "aws_s3_bucket_versioning" "lambdas" {
  bucket = aws_s3_bucket.lambdas.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_acl" "lambdas" {
  bucket = aws_s3_bucket.lambdas.id
  acl    = "private"
}

resource "aws_s3_object" "update_nacl" {
  key    = "update_nacl.zip"
  bucket = aws_s3_bucket.lambdas.id
  source = data.archive_file.update_nacl.output_path
  etag   = data.archive_file.update_nacl.output_md5
}

module "eventbridge" {
  source  = "terraform-aws-modules/eventbridge/aws"
  version = "1.14.2"

  create_bus = false

  rules = {
    crons = {
      description         = "Trigger update_nacl Lambda"
      schedule_expression = "rate(1 minute)"
    }
  }

  targets = {
    crons = [
      {
        name = "update_nacl"
        arn  = aws_lambda_function.update_nacl.arn
      }
    ]
  }
}
