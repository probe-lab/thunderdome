resource "aws_iam_role" "ecsTaskExecutionRole" {
  name               = "ecsTaskExecutionRole"
  assume_role_policy = data.aws_iam_policy_document.assume_role_policy.json
}

data "aws_iam_policy_document" "assume_role_policy" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy_attachment" "ecsTaskExecutionRole_default_policy" {
  role       = aws_iam_role.ecsTaskExecutionRole.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "ecsTaskExecutionRole_secretsmanager" {
  statement {
    actions = ["kms:Decrypt", "secretsmanager:GetSecretValue"]
    resources = [
      data.aws_secretsmanager_secret.prometheus-secret.arn,
      data.aws_secretsmanager_secret.dealgood-loki-secret.arn,
      data.aws_kms_key.default_secretsmanager_key.arn,
    ]
  }
}

resource "aws_iam_policy" "ecsTaskExecutionRole_policy" {
  policy = data.aws_iam_policy_document.ecsTaskExecutionRole_secretsmanager.json
}

resource "aws_iam_role_policy_attachment" "ecsTaskExecutionRole_policy" {
  role       = aws_iam_role.ecsTaskExecutionRole.name
  policy_arn = aws_iam_policy.ecsTaskExecutionRole_policy.arn
}

resource "aws_iam_policy" "ssm-exec" {
  name = "ssm-exec"
  path = "/"

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

resource "aws_iam_role" "target" {
  name = "target"
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
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "target-ssm" {
  role       = aws_iam_role.target.name
  policy_arn = aws_iam_policy.ssm-exec.arn
}


resource "aws_iam_role" "dealgood" {
  name = "dealgood"
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

resource "aws_iam_role_policy_attachment" "dealgood-ssm" {
  role       = aws_iam_role.dealgood.name
  policy_arn = aws_iam_policy.ssm-exec.arn
}

resource "aws_iam_role_policy_attachment" "dealgood_sqs_subscribe" {
  role       = aws_iam_role.dealgood.name
  policy_arn = aws_iam_policy.sqs_subscribe.arn
}


resource "aws_iam_role" "skyfish" {
  name = "skyfish"
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
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "skyfish-sns" {
  role       = aws_iam_role.skyfish.name
  policy_arn = aws_iam_policy.sns_publish.arn
}


resource "aws_iam_role" "ironbar" {
  name = "ironbar"
  inline_policy {
    name = "ironbar_inline"
    policy = jsonencode({
      "Version" : "2012-10-17",
      "Statement" : [
        {
          "Sid" : "ironbar",
          "Effect" : "Allow",
          "Action" : [
            "dynamodb:BatchGetItem",
            "dynamodb:BatchWriteItem",
            "dynamodb:PutItem",
            "dynamodb:DescribeTable",
            "dynamodb:DeleteItem",
            "dynamodb:GetItem",
            "dynamodb:Scan",
            "dynamodb:Query",
            "dynamodb:UpdateItem",
            "dynamodb:UpdateTable",
            "ecs:DescribeTasks",
            "ecs:DescribeTaskDefinition",
            "ecs:DeregisterTaskDefinition",
            "sns:GetSubscriptionAttributes",
            "ecs:StopTask",
            "sns:Unsubscribe",
            "sqs:DeleteQueue",
            "sqs:GetQueueAttributes"
          ],
          "Resource" : "*"
        }
      ]
    })
  }
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
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })
}

resource "aws_iam_user" "deployer" {
  for_each = toset(local.deployers)
  name     = each.key
}

resource "aws_iam_group" "deployers" {
  name = "deployers"
}


resource "aws_iam_user_group_membership" "deployer" {
  for_each = aws_iam_user.deployer
  user     = each.value.name

  groups = [
    aws_iam_group.deployers.name,
  ]
}

resource "aws_iam_group_policy" "deployers" {
  name  = "deployers"
  group = aws_iam_group.deployers.name
  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Sid" : "ironbar",
        "Effect" : "Allow",
        "Action" : [
          "ec2:DescribeInstances",
          "ecr:BatchCheckLayerAvailability",
          "ecr:CompleteLayerUpload",
          "ecr:DescribeImages",
          "ecr:GetAuthorizationToken",
          "ecr:UploadLayerPart",
          "ecr:InitiateLayerUpload",
          "ecr:PutImage",
          "ecs:DeregisterTaskDefinition",
          "ecs:DescribeClusters",
          "ecs:DescribeTasks",
          "ecs:DescribeTaskDefinition",
          "ecs:DescribeContainerInstances",
          "ecs:RegisterTaskDefinition",
          "ecs:RunTask",
          "ecs:StopTask",
          "s3:GetObject",
          "sns:GetSubscriptionAttributes",
          "sns:Subscribe",
          "sns:Unsubscribe",
          "sqs:CreateQueue",
          "sqs:DeleteQueue",
          "sqs:GetQueueAttributes",
          "sqs:SetQueueAttributes"
        ],
        "Resource" : "*"
      }
    ]
  })
}

resource "aws_iam_policy" "testbox_policy" {
  name        = "testbox-policy"
  path        = "/"
  description = "Policy for providing permissions to user test boxes"

  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Action" : [
          "ssm:GetParameters",
          "ssm:GetParameter",
        ],
        "Resource" : "*"
      },
      {
        "Effect" : "Allow",
        "Action" : [
          "s3:GetObject",
          "s3:List",
        ],
        "Resource" : "*"
      }
    ]
  })

}

resource "aws_iam_role" "testbox_role" {
  name = "testbox-role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "testbox_policy_role" {
  role       = aws_iam_role.testbox_role.name
  policy_arn = aws_iam_policy.testbox_policy.arn
}

resource "aws_iam_role_policy_attachment" "testbox_sns_publish" {
  role       = aws_iam_role.testbox_role.name
  policy_arn = aws_iam_policy.sns_publish.arn
}

resource "aws_iam_role_policy_attachment" "testbox_sns_subscribe" {
  role       = aws_iam_role.testbox_role.name
  policy_arn = aws_iam_policy.sns_subscribe.arn
}

resource "aws_iam_role_policy_attachment" "testbox_sqs_subscribe" {
  role       = aws_iam_role.testbox_role.name
  policy_arn = aws_iam_policy.sqs_subscribe.arn
}

resource "aws_iam_instance_profile" "testbox_profile" {
  name = "testbox-profile"
  role = aws_iam_role.testbox_role.name
}


resource "aws_iam_policy" "sns_publish" {
  name = "sns-publish"
  path = "/"

  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Action" : [
          "sns:Publish",
        ],
        "Resource" : "${aws_sns_topic.gateway_requests.arn}"
      }
    ]
  })
}

resource "aws_iam_policy" "sns_subscribe" {
  name = "sns-subscribe"
  path = "/"

  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Action" : [
          "sns:Subscribe",
        ],
        "Resource" : "${aws_sns_topic.gateway_requests.arn}"
      }
    ]
  })
}

resource "aws_iam_policy" "sqs_subscribe" {
  name = "sqs-subscribe"
  path = "/"

  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Action" : [
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:ChangeMessageVisibility",
          "sqs:GetQueueAttributes",
          "sqs:GetQueueUrl",
          "sqs:ListQueues",
          "sqs:ListQueueTags",
        ],
        "Resource" : "*"
      }
    ]
  })
}


