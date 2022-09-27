package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	// "github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
)

type Target struct {
	name        string
	experiment  string
	image       string
	environment map[string]string

	mu                         sync.Mutex
	ready                      bool
	taskRoleResourceName       string
	taskDefinitionArn          string
	serviceDiscoveryServiceArn string
	ecsServiceArn              string
}

func NewTarget(name, experiment, image string, environment map[string]string) *Target {
	return &Target{
		experiment:  experiment,
		name:        name,
		image:       image,
		environment: environment,
	}
}

func (t *Target) Setup(ctx context.Context, base *BaseInfra) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(base.AwsRegion()),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	if err := t.setupTaskDefinition(ctx, base, sess); err != nil {
		return fmt.Errorf("task definition: %w", err)
	}

	if err := t.setupServiceDiscoveryService(ctx, base, sess); err != nil {
		return fmt.Errorf("service discovery service: %w", err)
	}

	if err := t.setupEcsService(ctx, base, sess); err != nil {
		return fmt.Errorf("ecs service: %w", err)
	}

	return nil
}

func (t *Target) setupTaskDefinition(ctx context.Context, base *BaseInfra, sess *session.Session) error {
	svc := ecs.New(sess)

	gwEnv := make([]*ecs.KeyValuePair, len(t.environment))
	for n, v := range t.environment {
		gwEnv = append(gwEnv, &ecs.KeyValuePair{Name: aws.String(n), Value: aws.String(v)})
	}

	in := &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(t.experiment + "-" + t.name),
		RequiresCompatibilities: []*string{aws.String("EC2")},
		NetworkMode:             aws.String("host"),
		ExecutionRoleArn:        aws.String(execution_role_arn),
		TaskRoleArn:             aws.String(base.TaskRoleArn()),
		Memory:                  aws.String("51200"), // TODO: review  50*1024
		Tags: []*ecs.Tag{
			{Key: aws.String("experiment"), Value: aws.String(t.experiment)},
			{Key: aws.String("target"), Value: aws.String(t.name)},
		},
		Volumes: []*ecs.Volume{
			{
				Name: aws.String("ipfs-data"),
			},
			{
				Name: aws.String("grafana-agent-data"),
			},
			{
				Name: aws.String("ecs-exporter-data"),
			},
			{
				Name: aws.String("efs"),
				EfsVolumeConfiguration: &ecs.EFSVolumeConfiguration{
					FileSystemId: aws.String(efs_file_system_id),
				},
			},
		},
		ContainerDefinitions: []*ecs.ContainerDefinition{
			{
				Name:        aws.String("gateway"),
				Image:       aws.String(t.image),
				Cpu:         aws.Int64(0),
				Essential:   aws.Bool(true),
				Environment: gwEnv,
				MountPoints: []*ecs.MountPoint{
					{
						SourceVolume:  aws.String("ipfs-data"),
						ContainerPath: aws.String("/data/ipfs"),
					},
					{
						SourceVolume:  aws.String("efs"),
						ContainerPath: aws.String("/mnt/efs"),
					},
				},
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver: aws.String("awslogs"),
					Options: map[string]*string{
						"awslogs-group":         aws.String(base.LogGroupName()),
						"awslogs-region":        aws.String(base.AwsRegion()),
						"awslogs-stream-prefix": aws.String("ecs"),
					},
				},
				PortMappings: []*ecs.PortMapping{
					{
						ContainerPort: aws.Int64(8080),
						HostPort:      aws.Int64(8080),
						Protocol:      aws.String("tcp"),
					},
				},
				Ulimits: []*ecs.Ulimit{
					{
						Name:      aws.String("nofile"),
						HardLimit: aws.Int64(1048576),
						SoftLimit: aws.Int64(1048576),
					},
				},
			},
			{
				Name:  aws.String("grafana-agent"),
				Image: aws.String("grafana/agent:v0.26.1"),
				Command: []*string{
					aws.String("-metrics.wal-directory=/data/grafana-agent"),
					aws.String("-config.expand-env"),
					aws.String("-enable-features=remote-configs"),
					aws.String("-config.file=" + grafana_agent_target_config_url), // TODO: different for dealgood

				},
				Environment: []*ecs.KeyValuePair{
					// we use these for setting labels on metrics
					{
						Name:  aws.String("THUNDERDOME_EXPERIMENT"),
						Value: aws.String(t.experiment),
					},
					{
						Name:  aws.String("THUNDERDOME_TARGET"),
						Value: aws.String(t.name),
					},
				},
				Essential: aws.Bool(true),
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver: aws.String("awslogs"),
					Options: map[string]*string{
						"awslogs-group":         aws.String(base.LogGroupName()),
						"awslogs-region":        aws.String(base.AwsRegion()),
						"awslogs-stream-prefix": aws.String("ecs"),
					},
				},
				MountPoints: []*ecs.MountPoint{
					{
						SourceVolume:  aws.String("grafana-agent-data"),
						ContainerPath: aws.String("/data/grafana-agent-data"),
					},
					{
						SourceVolume:  aws.String("efs"),
						ContainerPath: aws.String("/mnt/efs"),
					},
				},
				Secrets: []*ecs.Secret{
					{
						Name:      aws.String("GRAFANA_USER"),
						ValueFrom: aws.String(grafana_push_secret_arn + ":username::"),
					},
					{
						Name:      aws.String("GRAFANA_PASS"),
						ValueFrom: aws.String(grafana_push_secret_arn + ":password::"),
					},
				},
			},
			{
				Name:      aws.String("ecs-exporter"),
				Image:     aws.String("quay.io/prometheuscommunity/ecs-exporter:v0.1.1"),
				Essential: aws.Bool(true),
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver: aws.String("awslogs"),
					Options: map[string]*string{
						"awslogs-group":         aws.String(base.LogGroupName()),
						"awslogs-region":        aws.String(base.AwsRegion()),
						"awslogs-stream-prefix": aws.String("ecs"),
					},
				},
				MountPoints: []*ecs.MountPoint{
					{
						SourceVolume:  aws.String("ecs-exporter-data"),
						ContainerPath: aws.String("/data/ecs-exporter-data"),
					},
					{
						SourceVolume:  aws.String("efs"),
						ContainerPath: aws.String("/mnt/efs"),
					},
				},
				PortMappings: []*ecs.PortMapping{
					{
						ContainerPort: aws.Int64(9779),
						HostPort:      aws.Int64(9779),
						Protocol:      aws.String("tcp"),
					},
				},
			},
		},
	}

	out, err := svc.RegisterTaskDefinition(in)
	if err != nil {
		return fmt.Errorf("create task definition: %w", err)
	}

	if out == nil || out.TaskDefinition == nil || out.TaskDefinition.TaskDefinitionArn == nil {
		return fmt.Errorf("no task definition arn found")
	}

	t.taskDefinitionArn = *out.TaskDefinition.TaskDefinitionArn
	return nil
}

func (t *Target) setupServiceDiscoveryService(ctx context.Context, base *BaseInfra, sess *session.Session) error {
	svc := servicediscovery.New(sess)

	in := &servicediscovery.CreateServiceInput{
		Name: aws.String(t.experiment + "-" + t.name),
		DnsConfig: &servicediscovery.DnsConfig{
			NamespaceId: aws.String(base.ServiceDiscoveryPrivateDnsNamespaceID()),
			DnsRecords: []*servicediscovery.DnsRecord{
				{
					TTL:  aws.Int64(10),
					Type: aws.String("SRV"),
				},
			},
			RoutingPolicy: aws.String("MULTIVALUE"),
		},
	}

	out, err := svc.CreateService(in)
	if err != nil {
		return fmt.Errorf("create service discovery service: %w", err)
	}

	if out == nil || out.Service == nil || out.Service.Arn == nil {
		return fmt.Errorf("no service discovery service arn found")
	}

	t.serviceDiscoveryServiceArn = *out.Service.Arn

	return nil
}

func (t *Target) setupEcsService(ctx context.Context, base *BaseInfra, sess *session.Session) error {
	svc := ecs.New(sess)

	in := &ecs.CreateServiceInput{
		ServiceName:          aws.String(t.experiment + "-" + t.name),
		Cluster:              aws.String(base.EcsClusterArn()),
		TaskDefinition:       aws.String(t.taskDefinitionArn),
		DesiredCount:         aws.Int64(1),
		EnableExecuteCommand: aws.Bool(true),
		ServiceRegistries: []*ecs.ServiceRegistry{
			{
				RegistryArn:   aws.String(t.serviceDiscoveryServiceArn),
				ContainerPort: aws.Int64(8080),
				ContainerName: aws.String("gateway"),
			},
		},

		CapacityProviderStrategy: []*ecs.CapacityProviderStrategyItem{
			{
				Base:             aws.Int64(0),
				CapacityProvider: aws.String("one"),
				Weight:           aws.Int64(1),
			},
		},
	}

	out, err := svc.CreateService(in)
	if err != nil {
		return fmt.Errorf("create service discovery service: %w", err)
	}

	if out == nil || out.Service == nil || out.Service.ServiceArn == nil {
		return fmt.Errorf("no ecs service arn found")
	}

	t.ecsServiceArn = *out.Service.ServiceArn
	return nil
}

func (t *Target) Teardown(ctx context.Context) error {
	panic("not implemented")
}

func (t *Target) Ready(ctx context.Context) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// sess, err := session.NewSession(&aws.Config{
	// 	Region: aws.String(aws_region),
	// })
	// if err != nil {
	// 	return false, fmt.Errorf("new session: %w", err)
	// }

	// svc := iam.New(sess)
	// in := &iam.GetRoleInput{
	// 	RoleName: aws.String(a.name),
	// }
	// out, err := svc.GetRole(in)
	// if err != nil {
	// 	if aerr, ok := err.(awserr.Error); ok {
	// 		switch aerr.Code() {
	// 		case iam.ErrCodeNoSuchEntityException:
	// 			return false, nil // ready check failed
	// 		default:
	// 			return false, fmt.Errorf("get role: %w", err)
	// 		}
	// 	}

	// 	return false, fmt.Errorf("get role: %w", err)
	// }

	// if out == nil || out.Role == nil || out.Role.Arn == nil {
	// 	return false, fmt.Errorf("no arn returned")
	// }

	t.ready = true
	return true, nil
}

func (t *Target) Status(ctx context.Context, base *BaseInfra) ([]ComponentStatus, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(base.AwsRegion()),
	})
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	statusFuncs := []struct {
		Name string
		Func func(context.Context, *session.Session) (bool, error)
	}{
		// {
		// 	Name: "task definition",
		// 	Func: t.readyTaskDefinition,
		// },
		// {
		// 	Name: "service discovery service",
		// 	Func: t.readyServiceDiscoveryService,
		// },
		// {
		// 	Name: "ecs service",
		// 	Func: t.readyEcsService,
		// },
	}

	statuses := []ComponentStatus{}

	for _, sf := range statusFuncs {
		status := ComponentStatus{
			Name: sf.Name,
		}
		status.Ready, status.Error = sf.Func(ctx, sess)
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// func (t *Target) statusTaskDefinition(ctx context.Context, base *BaseInfra, sess *session.Session) (bool, error) {
// 	svc := ecs.New(sess)

// 	in := &ecs.ListTaskDefinitionsInput{

// 	}

// 	out, err := ecs.ListTaskDefinitions(in)
// 	if err != nil {

// 	}

// }
