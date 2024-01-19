locals {
  agent_configs = toset([
    "dealgood",
    "skyfish",
    "target",
    "ironbar"
  ])
}
resource "aws_iam_user" "grafana" {
  name = "grafana"
}

resource "aws_iam_user_policy_attachment" "grafana" {
  user       = aws_iam_user.grafana.name
  policy_arn = aws_iam_policy.grafana.arn
}

resource "aws_iam_policy" "grafana" {
  name   = "grafana"
  path   = "/"
  policy = data.aws_iam_policy_document.grafana.json
}

data "aws_iam_policy_document" "grafana" {
  statement {
    sid = "AllowReadingMetricsFromCloudWatch"
    actions = [
      "cloudwatch:DescribeAlarmsForMetric",
      "cloudwatch:DescribeAlarmHistory",
      "cloudwatch:DescribeAlarms",
      "cloudwatch:ListMetrics",
      "cloudwatch:GetMetricStatistics",
      "cloudwatch:GetMetricData",
      "cloudwatch:GetInsightRuleReport"
    ]
    resources = ["*"]
  }

  statement {
    sid = "AllowReadingLogsFromCloudWatch"
    actions = [
      "logs:DescribeLogGroups",
      "logs:GetLogGroupFields",
      "logs:StartQuery",
      "logs:StopQuery",
      "logs:GetQueryResults",
      "logs:GetLogEvents"
    ]

    resources = ["*"]
  }

  statement {
    sid = "AllowReadingTagsInstancesRegionsFromEC2"
    actions = [
      "ec2:DescribeTags",
      "ec2:DescribeInstances",
      "ec2:DescribeRegions"
    ]
    resources = ["*"]
  }

  statement {
    sid       = "AllowReadingResourcesForTags"
    actions   = ["tag:GetResources"]
    resources = ["*"]
  }
}


module "grafana_agent_config" {
  for_each = local.agent_configs
  source   = "terraform-aws-modules/s3-bucket/aws//modules/object"

  acl    = "public-read"
  bucket = module.s3_bucket_public.s3_bucket_id
  key    = "grafana-agent-config/${each.key}.yaml"

  file_source = "./files/grafana-agent-config/${each.key}.yaml"
  # ensure changes to local file are detected and then uploaded
  etag = filemd5("./files/grafana-agent-config/${each.key}.yaml")
}

