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
	PrometheusSecretArn           string
	IronbarAddr                   string
	LogGroupName                  string
	RequestSNSTopicArn            string
	TargetGrafanaAgentConfigURL   string
	TargetTaskRoleArn             string
	VpcPublicSubnet               string
	CapacityProviders             map[string]CapacityProvider // currently staticly setup
}

type CapacityProvider struct {
	Name         string
	InstanceType InstanceType
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

	// TODO: read from json
	base.setupCapacityProviders()

	return base, nil
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

type InstanceType struct {
	Name        string
	MaxMemory   int // in gigabytes
	MaxCPU      int // in cores
	CostPerHour int // in cents
}

func (b *BaseInfra) setupCapacityProviders() {
	// These are defined in terraform
	b.CapacityProviders = map[string]CapacityProvider{
		"compute_large": {
			Name: "compute_large",
			InstanceType: InstanceType{
				Name:        "c6id.8xlarge",
				MaxMemory:   64,
				MaxCPU:      32,
				CostPerHour: 161,
			},
		},
		"compute_medium": {
			Name: "compute_medium",
			InstanceType: InstanceType{
				Name:        "c6id.4xlarge",
				MaxMemory:   32,
				MaxCPU:      16,
				CostPerHour: 81,
			},
		},
		"compute_small": {
			Name: "compute_small",
			InstanceType: InstanceType{
				Name:        "c6id.2xlarge",
				MaxMemory:   16,
				MaxCPU:      8,
				CostPerHour: 40,
			},
		},
		"io_large": {
			Name: "io_large",
			InstanceType: InstanceType{
				Name:        "i3en.2xlarge",
				MaxMemory:   64,
				MaxCPU:      8,
				CostPerHour: 63,
			},
		},
		"io_medium": {
			Name: "io_medium",
			InstanceType: InstanceType{
				Name:        "i3en.xlarge",
				MaxMemory:   32,
				MaxCPU:      4,
				CostPerHour: 31,
			},
		},
	}
}
