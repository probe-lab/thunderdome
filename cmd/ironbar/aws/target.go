package aws

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
)

type Target struct {
	name        string
	experiment  string
	base        *BaseInfra
	image       string
	environment map[string]string
	awsRegion   string

	taskDefinitionFamily        string
	taskDefinitionRevision      int64
	serviceDiscoveryServiceName string
	ecsServiceName              string

	mu                         sync.Mutex
	ready                      bool
	taskRoleResourceName       string
	taskDefinitionArn          string
	serviceDiscoveryServiceArn string
	serviceDiscoveryServiceId  string
	ecsServiceArn              string
}

func NewTarget(name, experiment string, base *BaseInfra, image string, environment map[string]string) *Target {
	return &Target{
		base:                        base,
		experiment:                  experiment,
		name:                        name,
		image:                       image,
		environment:                 environment,
		taskDefinitionFamily:        experiment + "-" + name,
		serviceDiscoveryServiceName: experiment + "-" + name,
		ecsServiceName:              experiment + "-" + name,
	}
}

func (t *Target) Name() string { return fmt.Sprintf("target %s", t.name) }

func (t *Target) Setup(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(t.base.AwsRegion()),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	return TaskSequence(ctx, sess, "target "+t.name,
		t.createTaskDefinition(),
		t.createServiceDiscoveryService(),
		t.createEcsService(),
	)
}

func (t *Target) Teardown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(t.base.AwsRegion()),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	return TaskSequence(ctx, sess, "target "+t.name,
		t.deleteEcsService(),
		t.deleteServiceDiscoveryService(),
		t.deregisterTaskDefinition(),
	)
}

func (t *Target) Ready(ctx context.Context) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(t.awsRegion),
	})
	if err != nil {
		return false, fmt.Errorf("new session: %w", err)
	}

	return CheckSequence(ctx, sess, "target "+t.name,
		t.taskDefinitionIsActive(),
		t.serviceDiscoveryServiceExists(),
		t.ecsServiceExists(),
		t.ecsServiceStatusIsActive(),
		t.ecsServiceHasRunningTask(),
	)
}

func (t *Target) findExistingTaskDefinition(sess *session.Session) error {
	t.taskDefinitionArn = ""
	t.taskDefinitionRevision = 0

	svc := ecs.New(sess)
	in := &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: aws.String(t.taskDefinitionFamily),
	}
	out, err := svc.DescribeTaskDefinition(in)
	if err != nil {
		aerr := &ecs.ClientException{}
		if errors.As(err, &aerr) {
			if strings.Contains(aerr.Error(), "Unable to describe task definition") {
				return nil
			}
		}
		return fmt.Errorf("describe task definition: %w", err)
	}

	if out == nil || out.TaskDefinition == nil || out.TaskDefinition.TaskDefinitionArn == nil || out.TaskDefinition.Status == nil || out.TaskDefinition.Revision == nil {
		return fmt.Errorf("unable to read task definition arn, status or revision")
	}

	if *out.TaskDefinition.Status == "ACTIVE" {
		t.taskDefinitionArn = *out.TaskDefinition.TaskDefinitionArn
		t.taskDefinitionRevision = *out.TaskDefinition.Revision
	}
	return nil
}

func (t *Target) createTaskDefinition() Task {
	return Task{
		Name:  "create task definition " + t.taskDefinitionFamily,
		Check: t.taskDefinitionIsActive(),
		Func: func(ctx context.Context, sess *session.Session) error {
			gwEnv := make([]*ecs.KeyValuePair, len(t.environment))
			for n, v := range t.environment {
				gwEnv = append(gwEnv, &ecs.KeyValuePair{Name: aws.String(n), Value: aws.String(v)})
			}

			in := &ecs.RegisterTaskDefinitionInput{
				Family:                  aws.String(t.taskDefinitionFamily),
				RequiresCompatibilities: []*string{aws.String("EC2")},
				NetworkMode:             aws.String("host"),
				ExecutionRoleArn:        aws.String(execution_role_arn),
				TaskRoleArn:             aws.String(t.base.TaskRoleArn()),
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
								"awslogs-group":         aws.String(t.base.LogGroupName()),
								"awslogs-region":        aws.String(t.base.AwsRegion()),
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
								"awslogs-group":         aws.String(t.base.LogGroupName()),
								"awslogs-region":        aws.String(t.base.AwsRegion()),
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
								"awslogs-group":         aws.String(t.base.LogGroupName()),
								"awslogs-region":        aws.String(t.base.AwsRegion()),
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

			svc := ecs.New(sess)
			out, err := svc.RegisterTaskDefinition(in)
			if err != nil {
				return fmt.Errorf("create task definition: %w", err)
			}

			if out == nil || out.TaskDefinition == nil || out.TaskDefinition.TaskDefinitionArn == nil {
				return fmt.Errorf("no task definition arn found")
			}

			t.taskDefinitionArn = *out.TaskDefinition.TaskDefinitionArn
			return nil
		},
	}
}

func (t *Target) deregisterTaskDefinition() Task {
	return Task{
		Name:  "deregister task definition " + t.taskDefinitionFamily,
		Check: t.taskDefinitionIsInactive(),
		Func: func(ctx context.Context, sess *session.Session) error {
			in := &ecs.DeregisterTaskDefinitionInput{
				TaskDefinition: aws.String(fmt.Sprintf("%s:%d", t.taskDefinitionFamily, t.taskDefinitionRevision)),
			}

			svc := ecs.New(sess)
			_, err := svc.DeregisterTaskDefinition(in)
			if err != nil {
				return fmt.Errorf("deregister task definition: %w", err)
			}

			return nil
		},
	}
}

func (t *Target) findExistingServiceDiscoveryService(ctx context.Context, sess *session.Session) error {
	t.serviceDiscoveryServiceArn = ""
	t.serviceDiscoveryServiceId = ""

	svc := servicediscovery.New(sess)

	in := &servicediscovery.ListServicesInput{
		Filters: []*servicediscovery.ServiceFilter{
			{
				Condition: aws.String("EQ"),
				Name:      aws.String("NAMESPACE_ID"),
				Values: []*string{
					aws.String(t.base.ServiceDiscoveryPrivateDnsNamespaceID()),
				},
			},
		},
	}

	out, err := svc.ListServices(in)
	if err != nil {
		return fmt.Errorf("list service discovery services: %w", err)
	}

	if out == nil {
		return fmt.Errorf("list service discovery services gave no result")
	}

	for _, ss := range out.Services {
		if ss.Name != nil && *ss.Name == t.serviceDiscoveryServiceName {
			if ss.Arn == nil || ss.Id == nil {
				return fmt.Errorf("no service discovery service arn or id found")
			}
			t.serviceDiscoveryServiceArn = *ss.Arn
			t.serviceDiscoveryServiceId = *ss.Id
			return nil
		}
	}

	if out.NextToken != nil {
		return fmt.Errorf("no service discovery service found but more were available (FIXME: implement pagination)")
	}

	return nil
}

func (t *Target) createServiceDiscoveryService() Task {
	return Task{
		Name:  "create service discovery service " + t.serviceDiscoveryServiceName,
		Check: t.serviceDiscoveryServiceExists(),
		Func: func(ctx context.Context, sess *session.Session) error {
			in := &servicediscovery.CreateServiceInput{
				Name: aws.String(t.serviceDiscoveryServiceName),
				DnsConfig: &servicediscovery.DnsConfig{
					NamespaceId: aws.String(t.base.ServiceDiscoveryPrivateDnsNamespaceID()),
					DnsRecords: []*servicediscovery.DnsRecord{
						{
							TTL:  aws.Int64(10),
							Type: aws.String("SRV"),
						},
					},
					RoutingPolicy: aws.String("MULTIVALUE"),
				},
			}

			svc := servicediscovery.New(sess)
			out, err := svc.CreateService(in)
			if err != nil {
				return fmt.Errorf("create service discovery service: %w", err)
			}

			if out == nil || out.Service == nil || out.Service.Arn == nil || out.Service.Id == nil {
				return fmt.Errorf("no service discovery service arn or id found")
			}

			t.serviceDiscoveryServiceArn = *out.Service.Arn
			t.serviceDiscoveryServiceId = *out.Service.Id

			return nil
		},
	}
}

func (t *Target) deleteServiceDiscoveryService() Task {
	return Task{
		Name:  "delete service discovery service " + t.serviceDiscoveryServiceName,
		Check: t.serviceDiscoveryServiceDoesNotExist(),
		Func: func(ctx context.Context, sess *session.Session) error {
			in := &servicediscovery.DeleteServiceInput{
				Id: aws.String(t.serviceDiscoveryServiceId),
			}

			svc := servicediscovery.New(sess)
			_, err := svc.DeleteService(in)
			if err != nil {
				return fmt.Errorf("delete service discovery service: %w", err)
			}

			return nil
		},
	}
}

func (t *Target) findExistingEcsService(ctx context.Context, sess *session.Session) error {
	t.ecsServiceArn = ""

	svc := ecs.New(sess)

	in := &ecs.DescribeServicesInput{
		Cluster: aws.String(t.base.EcsClusterArn()),
		Services: []*string{
			aws.String(t.ecsServiceName),
		},
	}

	out, err := svc.DescribeServices(in)
	if err != nil {
		return fmt.Errorf("describe services: %w", err)
	}

	// TODO: check out.Failures
	for _, s := range out.Services {
		if s.ServiceName != nil && *s.ServiceName == t.ecsServiceName {
			if s.Status == nil {
				return fmt.Errorf("unable to read ecs service status")
			}
			if s.ServiceArn == nil {
				return fmt.Errorf("unable to read ecs service arn")
			}
			if *s.Status == "ACTIVE" {
				t.ecsServiceArn = *s.ServiceArn
			}
			return nil
		}
	}

	return nil
}

func (t *Target) createEcsService() Task {
	return Task{
		Name:  "create ecs service " + t.ecsServiceName,
		Check: t.ecsServiceExists(),
		Func: func(ctx context.Context, sess *session.Session) error {
			if err := t.findExistingEcsService(ctx, sess); err != nil {
				return fmt.Errorf("find existing ecs service: %w", err)
			}

			if t.ecsServiceArn != "" {
				// TODO: don't assume this is configured how we want it to be
				log.Printf("target %q ecs service: already exists with arn %s", t.name, t.ecsServiceArn)
				return nil
			}

			svc := ecs.New(sess)

			in := &ecs.CreateServiceInput{
				ServiceName:          aws.String(t.ecsServiceName),
				Cluster:              aws.String(t.base.EcsClusterArn()),
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
				return fmt.Errorf("create service: %w", err)
			}

			if out == nil || out.Service == nil || out.Service.ServiceArn == nil {
				return fmt.Errorf("no ecs service arn found")
			}

			t.ecsServiceArn = *out.Service.ServiceArn
			return nil
		},
	}
}

func (t *Target) stopAllTasks() Task {
	return Task{
		Name:  "ecs service: stop all tasks",
		Check: t.ecsServiceHasNoRunningTasks(),
		Func: func(ctx context.Context, sess *session.Session) error {
			svc := ecs.New(sess)
			// reduce running task count
			uin := &ecs.UpdateServiceInput{
				Cluster:      aws.String(t.base.EcsClusterArn()),
				Service:      aws.String(t.ecsServiceName),
				DesiredCount: aws.Int64(0),
			}
			uout, err := svc.UpdateService(uin)
			if err != nil {
				return fmt.Errorf("update service: %w", err)
			}
			if uout == nil || uout.Service == nil || uout.Service.DesiredCount == nil || *uout.Service.DesiredCount != 0 {
				return fmt.Errorf("could not set desired count to 0")
			}
			return nil
		},
	}
}

func (t *Target) deleteEcsService() Task {
	return Task{
		Name:  "ecs service: delete",
		Check: t.ecsServiceStatusIsInactive(),
		Func: func(ctx context.Context, sess *session.Session) error {
			if err := ExecuteTask(ctx, sess, "", t.stopAllTasks()); err != nil {
				return fmt.Errorf("stop tasks: %w", err)
			}

			svc := ecs.New(sess)
			// reduce running task count
			din := &ecs.DeleteServiceInput{
				Cluster: aws.String(t.base.EcsClusterArn()),
				Service: aws.String(t.ecsServiceName),
			}
			if _, err := svc.DeleteService(din); err != nil {
				return fmt.Errorf("delete service: %w", err)
			}
			return nil
		},
	}
}

func (t *Target) ecsServiceHasNoRunningTasks() Check {
	return Check{
		Name:        "ecs service has no running tasks",
		FailureText: "ecs service has running tasks",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			svc := ecs.New(sess)

			in := &ecs.DescribeServicesInput{
				Cluster: aws.String(t.base.EcsClusterArn()),
				Services: []*string{
					aws.String(t.ecsServiceName),
				},
			}

			out, err := svc.DescribeServices(in)
			if err != nil {
				return false, fmt.Errorf("describe services: %w", err)
			}
			for _, s := range out.Services {
				if s.ServiceName != nil && *s.ServiceName == t.ecsServiceName {
					if s.RunningCount != nil && *s.RunningCount == 0 {
						return true, nil
					}
					return false, nil
				}
			}

			return false, nil
		},
	}
}

func (t *Target) ecsServiceHasRunningTask() Check {
	return Check{
		Name: "ecs service has a running task",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			svc := ecs.New(sess)

			in := &ecs.DescribeServicesInput{
				Cluster: aws.String(t.base.EcsClusterArn()),
				Services: []*string{
					aws.String(t.ecsServiceName),
				},
			}

			out, err := svc.DescribeServices(in)
			if err != nil {
				return false, fmt.Errorf("describe services: %w", err)
			}
			for _, s := range out.Services {
				if s.ServiceName != nil && *s.ServiceName == t.ecsServiceName {
					if s.RunningCount != nil && *s.RunningCount == 1 {
						return true, nil
					}
					return false, nil
				}
			}

			return false, nil
		},
	}
}

func (t *Target) ecsServiceStatusIsInactive() Check {
	return Check{
		Name: "ecs service is inactive",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			svc := ecs.New(sess)

			in := &ecs.DescribeServicesInput{
				Cluster: aws.String(t.base.EcsClusterArn()),
				Services: []*string{
					aws.String(t.ecsServiceName),
				},
			}

			out, err := svc.DescribeServices(in)
			if err != nil {
				return false, fmt.Errorf("describe services: %w", err)
			}
			for _, s := range out.Services {
				if s.ServiceName != nil && *s.ServiceName == t.ecsServiceName {
					if s.Status != nil && *s.Status == "INACTIVE" {
						return true, nil
					}
					return false, nil
				}
			}

			// Service does not exist
			return true, nil
		},
	}
}

func (t *Target) ecsServiceStatusIsActive() Check {
	return Check{
		Name: "ecs service is active",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			svc := ecs.New(sess)

			in := &ecs.DescribeServicesInput{
				Cluster: aws.String(t.base.EcsClusterArn()),
				Services: []*string{
					aws.String(t.ecsServiceName),
				},
			}

			out, err := svc.DescribeServices(in)
			if err != nil {
				return false, fmt.Errorf("describe services: %w", err)
			}
			for _, s := range out.Services {
				if s.ServiceName != nil && *s.ServiceName == t.ecsServiceName {
					if s.Status != nil && *s.Status == "ACTIVE" {
						return true, nil
					}
					return false, nil
				}
			}

			return false, nil
		},
	}
}

func (t *Target) taskDefinitionIsActive() Check {
	return Check{
		Name: "task definition is active",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			if err := t.findExistingTaskDefinition(sess); err != nil {
				return false, err
			}

			return t.taskDefinitionArn != "", nil
		},
	}
}

func (t *Target) taskDefinitionIsInactive() Check {
	return Check{
		Name:        "task definition is inactive",
		FailureText: "task definition is active",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			if err := t.findExistingTaskDefinition(sess); err != nil {
				return false, err
			}

			return t.taskDefinitionArn == "", nil
		},
	}
}

func (t *Target) serviceDiscoveryServiceExists() Check {
	return Check{
		Name:        "service discovery service exists",
		FailureText: "service discovery service does not exist",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			if err := t.findExistingServiceDiscoveryService(ctx, sess); err != nil {
				return false, err
			}

			return t.serviceDiscoveryServiceArn != "" && t.serviceDiscoveryServiceId != "", nil
		},
	}
}

func (t *Target) serviceDiscoveryServiceDoesNotExist() Check {
	return Check{
		Name:        "service discovery service does not exist",
		FailureText: "service discovery service exists",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			if err := t.findExistingServiceDiscoveryService(ctx, sess); err != nil {
				return false, err
			}

			return t.serviceDiscoveryServiceArn == "" || t.serviceDiscoveryServiceId == "", nil
		},
	}
}

func (t *Target) ecsServiceExists() Check {
	return Check{
		Name:        "ecs service exists",
		FailureText: "ecs service does not exist",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			if err := t.findExistingEcsService(ctx, sess); err != nil {
				return false, err
			}
			return t.ecsServiceArn != "", nil
		},
	}
}

func (t *Target) ecsServiceDoesNotExist() Check {
	return Check{
		Name:        "ecs service does not exist",
		FailureText: "ecs service exists",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			if err := t.findExistingEcsService(ctx, sess); err != nil {
				return false, err
			}
			return t.ecsServiceArn == "", nil
		},
	}
}
