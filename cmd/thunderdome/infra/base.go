package infra

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/s3"
	"golang.org/x/exp/slog"
)

type BaseInfra struct {
	AwsRegion                     string
	DealgoodGrafanaAgentConfigURL string
	DealgoodImage                 string
	DealgoodSecurityGroup         string
	DealgoodTaskRoleArn           string
	EcrBaseURL                    string
	EcsClusterArn                 string
	EcsExecutionRoleArn           string
	EfsFileSystemID               string
	ExperimentsTableName          string
	GrafanaPushSecretArn          string
	IronbarAddr                   string
	LogGroupName                  string
	RequestSNSTopicArn            string
	TargetGrafanaAgentConfigURL   string
	TargetTaskRoleArn             string
	VpcPublicSubnet               string
}

func NewBaseInfra(awsRegion string) (*BaseInfra, error) {
	// This file is written by terraform
	const bucket = "pl-thunderdome-private"
	const key = "infra.json"

	base := new(BaseInfra)
	logger := slog.With("component", base.Name())
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion),
	})
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	logger.Debug("initializing from s3", "region", awsRegion, "bucket", bucket, "key", key)
	svc := s3.New(sess)
	in := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	out, err := svc.GetObject(in)
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	defer out.Body.Close()

	dec := json.NewDecoder(out.Body)
	if err := dec.Decode(base); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}

	base.IronbarAddr = "127.0.0.1:8321"

	return base, nil

	// return &BaseInfra{
	// 	EcsClusterArn:                 "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome",
	// 	LogGroupName:                  "thunderdome",
	// 	EcsExecutionRoleArn:           "arn:aws:iam::147263665150:role/ecsTaskExecutionRole",
	// 	TargetTaskRoleArn:             "arn:aws:iam::147263665150:role/target",
	// 	DealgoodTaskRoleArn:           "arn:aws:iam::147263665150:role/dealgood",
	// 	EfsFileSystemID:               "fs-006bd3d793700a2df",
	// 	RequestSNSTopicArn:            "arn:aws:sns:eu-west-1:147263665150:gateway-requests",
	// 	GrafanaAgentTargetConfigURL:   "https://pl-thunderdome-public.s3.eu-west-1.amazonaws.com/grafana-agent-config/target.yaml",
	// 	GrafanaAgentDealgoodConfigURL: "https://pl-thunderdome-public.s3.eu-west-1.amazonaws.com/grafana-agent-config/dealgood.yaml",
	// 	GrafanaPushSecretArn:          "arn:aws:secretsmanager:eu-west-1:147263665150:secret:grafana-push-MxjNiv",
	// 	VpcPublicSubnet:               "subnet-04b1073d060c42a2f",
	// 	DealgoodSecurityGroup:         "sg-08cd1cfcd73b3f0ea",
	// 	DealgoodImage:                 "147263665150.dkr.ecr.eu-west-1.amazonaws.com/dealgood:2022-09-15__1504",
	// 	EcrBaseURL:                    "147263665150.dkr.ecr.eu-west-1.amazonaws.com",
	// }
}

func (b *BaseInfra) Name() string {
	return "base infra"
}

func (b *BaseInfra) Verify(ctx context.Context) error {
	slog.Info("verifying", "component", b.Name())
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(b.AwsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	ready, err := CheckSequence(ctx, sess, b.Name(),
		b.ecsClusterExists(),
		// TODO: check all base infra
	)
	if err != nil {
		return fmt.Errorf("failed to execute all checks: %w", err)
	}
	if !ready {
		return fmt.Errorf("failed to verify one or more components")
	}

	return nil
}

func (b *BaseInfra) ecsClusterExists() Check {
	return Check{
		Name:        "ecs cluster exists",
		FailureText: "ecs cluster does not exist",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			logger := slog.With("component", b.Name())

			logger.Debug("finding ecs cluster", "arn", b.EcsClusterArn)
			svc := ecs.New(sess)
			in := &ecs.DescribeClustersInput{
				Clusters: []*string{aws.String(b.EcsClusterArn)},
			}
			out, err := svc.DescribeClusters(in)
			if err != nil {
				return false, err
			}

			if out == nil || len(out.Clusters) == 0 {
				logger.Debug("no ecs clusters returned")
				return false, nil
			}

			for _, c := range out.Clusters {
				if c.ClusterArn == nil {
					continue
				}
				if *c.ClusterArn == b.EcsClusterArn {
					return true, nil
				}
			}
			logger.Debug("no ecs clusters matched expected cluster arn")
			return false, nil
		},
	}
}
