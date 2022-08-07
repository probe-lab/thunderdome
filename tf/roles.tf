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
      data.aws_secretsmanager_secret.grafana-push-secret.arn,
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
