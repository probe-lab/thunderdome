locals {
  dealgood_image_tag = "2022-09-15__1504"

  skyfish_image_tag = "2022-09-15__1542"

  ironbar_image_tag = "2023-02-23-11cccec"
  ironbar_port_number = 8321

  infra_json = jsonencode({
    AwsRegion                       = data.aws_region.current.name
    DealgoodGrafanaAgentConfigURL   = "http://${module.s3_bucket_public.s3_bucket_bucket_domain_name}/${module.grafana_agent_config["dealgood"].s3_object_id}"
    DealgoodImage                   = "${aws_ecr_repository.dealgood.repository_url}:${local.dealgood_image_tag}"
    DealgoodSecurityGroup           = aws_security_group.dealgood.id
    DealgoodTaskRoleArn             = aws_iam_role.dealgood.arn
    EcrBaseURL                      = aws_ecr_repository.thunderdome.repository_url
    EcsClusterArn                   = module.ecs-asg.cluster_id
    EcsExecutionRoleArn             = aws_iam_role.ecsTaskExecutionRole.arn
    EfsFileSystemID                 = aws_efs_file_system.thunderdome.id
    ExperimentsTableName            = aws_dynamodb_table.experiments.name
    GrafanaPushSecretArn            = data.aws_secretsmanager_secret.grafana-push-secret.arn
    IronbarAddr                     = "${aws_eip.ecs[0].public_ip}:${local.ironbar_port_number}"
    LogGroupName                    = aws_cloudwatch_log_group.logs.name
    RequestSNSTopicArn              = aws_sns_topic.gateway_requests.arn
    TargetGrafanaAgentConfigURL     = "http://${module.s3_bucket_public.s3_bucket_bucket_domain_name}/${module.grafana_agent_config["target"].s3_object_id}"
    TargetTaskRoleArn               = aws_iam_role.target.arn
    VpcPublicSubnet                 = module.vpc.public_subnets[0]
  })
}
