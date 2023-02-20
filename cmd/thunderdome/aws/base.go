package aws

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/exp/slog"
)

type BaseInfra struct {
	experiment                    string
	awsRegion                     string
	ecsClusterArn                 string
	logGroupName                  string
	ecsExecutionRoleArn           string
	targetTaskRoleArn             string
	dealgoodTaskRoleArn           string
	efsFileSystemID               string
	requestSNSTopicArn            string
	grafanaAgentTargetConfigURL   string
	grafanaAgentDealgoodConfigUrl string
	grafanaPushSecretArn          string
	vpcPublicSubnet               string
	dealgoodSecurityGroup         string
	dealgoodImage                 string
	ecrBaseURL                    string

	mu    sync.Mutex
	ready bool
}

func NewBaseInfra(experiment, awsRegion string) *BaseInfra {
	return &BaseInfra{
		experiment: experiment,
		awsRegion:  awsRegion,

		// TODO: get terraform to write these to an s3 bucket and read
		ecsClusterArn:                 "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome",
		logGroupName:                  "thunderdome",
		ecsExecutionRoleArn:           "arn:aws:iam::147263665150:role/ecsTaskExecutionRole",
		targetTaskRoleArn:             "arn:aws:iam::147263665150:role/target",
		dealgoodTaskRoleArn:           "arn:aws:iam::147263665150:role/dealgood",
		efsFileSystemID:               "fs-006bd3d793700a2df",
		requestSNSTopicArn:            "arn:aws:sns:eu-west-1:147263665150:gateway-requests",
		grafanaAgentTargetConfigURL:   "https://pl-thunderdome-public.s3.eu-west-1.amazonaws.com/grafana-agent-config/target.yaml",
		grafanaAgentDealgoodConfigUrl: "https://pl-thunderdome-public.s3.eu-west-1.amazonaws.com/grafana-agent-config/dealgood.yaml",
		grafanaPushSecretArn:          "arn:aws:secretsmanager:eu-west-1:147263665150:secret:grafana-push-MxjNiv",
		vpcPublicSubnet:               "subnet-04b1073d060c42a2f",
		dealgoodSecurityGroup:         "sg-08cd1cfcd73b3f0ea",
		dealgoodImage:                 "147263665150.dkr.ecr.eu-west-1.amazonaws.com/dealgood:2022-09-15__1504",
		ecrBaseURL:                    "147263665150.dkr.ecr.eu-west-1.amazonaws.com",
	}
}

func (b *BaseInfra) Name() string {
	return "base infra"
}

func (b *BaseInfra) AwsRegion() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.awsRegion
}

func (b *BaseInfra) LogGroupName() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.logGroupName
}

func (b *BaseInfra) EcsClusterArn() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ecsClusterArn
}

func (b *BaseInfra) EcsExecutionRoleArn() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ecsExecutionRoleArn
}

func (b *BaseInfra) TargetTaskRoleArn() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.targetTaskRoleArn
}

func (b *BaseInfra) DealgoodTaskRoleArn() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dealgoodTaskRoleArn
}

func (b *BaseInfra) EfsFileSystemID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.efsFileSystemID
}

func (b *BaseInfra) GrafanaPushSecretArn() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.grafanaPushSecretArn
}

func (b *BaseInfra) VpcPublicSubnet() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.vpcPublicSubnet
}

func (b *BaseInfra) DealgoodSecurityGroup() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dealgoodSecurityGroup
}

func (b *BaseInfra) RequestSNSTopicArn() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.requestSNSTopicArn
}

func (b *BaseInfra) GrafanaAgentTargetConfigURL() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.grafanaAgentTargetConfigURL
}

func (b *BaseInfra) GrafanaAgentDealgoodConfigURL() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.grafanaAgentDealgoodConfigUrl
}

func (b *BaseInfra) DealgoodImage() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dealgoodImage
}

func (b *BaseInfra) ECRBaseURL() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ecrBaseURL
}

func (b *BaseInfra) DealgoodEnvironment() map[string]string {
	return nil
}

func (b *BaseInfra) Setup(ctx context.Context) error {
	slog.Info("starting setup", "component", b.Name())
	if err := b.InspectExisting(ctx); err != nil {
		return fmt.Errorf("inspect existing infra: %w", err)
	}
	return nil
}

func (b *BaseInfra) InspectExisting(ctx context.Context) error {
	// TODO: check ecs cluster exists
	return nil
}

func (b *BaseInfra) Teardown(ctx context.Context) error {
	slog.Info("starting teardown", "component", b.Name())
	if err := b.InspectExisting(ctx); err != nil {
		return fmt.Errorf("inspect existing infra: %w", err)
	}
	return nil
}

func (b *BaseInfra) Ready(ctx context.Context) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.InspectExisting(ctx); err != nil {
		return false, fmt.Errorf("inspect existing infra: %w", err)
	}

	return true, nil
}
