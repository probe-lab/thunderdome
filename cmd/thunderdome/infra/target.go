package infra

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"

	// "github.com/aws/aws-sdk-go/service/servicediscovery"
	"golang.org/x/exp/slog"

	"github.com/probe-lab/thunderdome/cmd/ironbar/api"
)

type Target struct {
	name             string
	experiment       string
	base             *BaseInfra
	image            string
	capacityProvider string
	environment      map[string]string

	taskDefinitionFamily string
	taskName             string

	// mu guards access to fields in block directly below
	mu                     sync.Mutex
	ready                  bool
	taskDefinitionArn      string
	taskDefinitionRevision int64
	taskArn                string
	taskEC2InstanceID      string
	taskPrivateIPAddress   string
}

func NewTarget(name, experiment string, base *BaseInfra, image string, capacityProvider string, environment map[string]string) *Target {
	return &Target{
		base:                 base,
		experiment:           experiment,
		name:                 name,
		image:                image,
		capacityProvider:     capacityProvider,
		environment:          environment,
		taskDefinitionFamily: experiment + "-" + name,
		taskName:             experiment + "-" + name,
	}
}

func (t *Target) Name() string { return t.name }

func (t *Target) ComponentName() string { return fmt.Sprintf("target %s", t.name) }

func (t *Target) TaskDefinitionArn() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready {
		return ""
	}
	return t.taskDefinitionArn
}

func (t *Target) TaskArn() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready {
		return ""
	}
	return t.taskArn
}

func (t *Target) EC2InstanceID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready {
		return ""
	}
	return t.taskEC2InstanceID
}

func (t *Target) PrivateIPAddress() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready {
		return ""
	}
	return t.taskPrivateIPAddress
}

func (t *Target) GatewayURL() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready {
		return ""
	}
	return "http://" + t.taskPrivateIPAddress + ":8080"
}

func (t *Target) Resources() []api.Resource {
	t.mu.Lock()
	defer t.mu.Unlock()

	var res []api.Resource
	res = append(res, api.Resource{
		Type: api.ResourceTypeEcsTask,
		Keys: map[string]string{
			api.ResourceKeyEcsClusterArn: t.base.EcsClusterArn,
			api.ResourceKeyArn:           t.taskArn,
		},
	})
	res = append(res, api.Resource{
		Type: api.ResourceTypeEcsTaskDefinition,
		Keys: map[string]string{
			api.ResourceKeyArn: t.taskDefinitionArn,
		},
	})
	res = append(res, api.Resource{
		Type: api.ResourceTypeEc2Instance,
		Keys: map[string]string{
			api.ResourceKeyEc2InstanceID: t.taskEC2InstanceID,
		},
	})
	return res
}

func (t *Target) tags() map[string]*string {
	return map[string]*string{
		"experiment": aws.String(t.experiment),
		"component":  aws.String(t.ComponentName()),
	}
}

func (t *Target) Setup(ctx context.Context) error {
	slog.Info("starting setup", "component", t.ComponentName())

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(t.base.AwsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	return TaskSequence(ctx, sess, t.ComponentName(),
		t.createTaskDefinition(),
		t.runTask(),
	)
}

func (t *Target) Teardown(ctx context.Context) error {
	slog.Info("starting teardown", "component", t.ComponentName())
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(t.base.AwsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	return TaskSequence(ctx, sess, t.ComponentName(),
		t.stopTask(),
		t.deregisterTaskDefinition(),
	)
}

func (t *Target) Ready(ctx context.Context) (bool, error) {
	t.mu.Lock()
	t.ready = false
	t.mu.Unlock()
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(t.base.AwsRegion),
	})
	if err != nil {
		return false, fmt.Errorf("new session: %w", err)
	}

	ready, err := CheckSequence(ctx, sess, t.ComponentName(),
		t.taskDefinitionIsActive(),
		t.taskIsRunning(),
	)
	if !ready || err != nil {
		return ready, err
	}

	t.mu.Lock()
	t.ready = true
	t.mu.Unlock()
	return true, nil
}

func (t *Target) createTaskDefinition() Task {
	return Task{
		Name:  "create task definition",
		Check: t.taskDefinitionIsActive(),
		Func: func(ctx context.Context, sess *session.Session) error {
			cp, ok := t.base.CapacityProviders[t.capacityProvider]
			if !ok {
				return fmt.Errorf("unsupported capacity provider %q for %s", t.capacityProvider, t.ComponentName())
			}

			additionalEnv := map[string]string{
				// TODO: any additional env?
			}

			logStreamPrefix := fmt.Sprintf("%s-%s", t.experiment, t.name)

			in := &ecs.RegisterTaskDefinitionInput{
				Family:                  aws.String(t.taskDefinitionFamily),
				RequiresCompatibilities: []*string{aws.String("EC2")},
				NetworkMode:             aws.String("host"),
				Memory:                  aws.String(strconv.Itoa(1024 * (cp.InstanceType.MaxMemory - 2))),
				ExecutionRoleArn:        aws.String(t.base.EcsExecutionRoleArn),
				TaskRoleArn:             aws.String(t.base.TargetTaskRoleArn),
				Tags:                    ecsTags(t.tags()),
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
							FileSystemId: aws.String(t.base.EfsFileSystemID),
						},
					},
				},
				ContainerDefinitions: []*ecs.ContainerDefinition{
					{
						Name:        aws.String("gateway"),
						Image:       aws.String(t.image),
						Cpu:         aws.Int64(0),
						Essential:   aws.Bool(true),
						Environment: mapsToKeyValuePair(t.environment, additionalEnv),
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
								"awslogs-group":         aws.String(t.base.LogGroupName),
								"awslogs-region":        aws.String(t.base.AwsRegion),
								"awslogs-stream-prefix": aws.String(logStreamPrefix),
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
						Image: aws.String("grafana/agent:v0.39.1"),
						Command: []*string{
							aws.String("-metrics.wal-directory=/data/grafana-agent"),
							aws.String("-config.expand-env"),
							aws.String("-enable-features=remote-configs"),
							aws.String("-config.file=" + t.base.TargetGrafanaAgentConfigURL),
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
								"awslogs-group":         aws.String(t.base.LogGroupName),
								"awslogs-region":        aws.String(t.base.AwsRegion),
								"awslogs-stream-prefix": aws.String(logStreamPrefix),
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
								Name:      aws.String("PROMETHEUS_URL"),
								ValueFrom: aws.String(t.base.PrometheusSecretArn + ":url::"),
							},
							{
								Name:      aws.String("PROMETHEUS_USER"),
								ValueFrom: aws.String(t.base.PrometheusSecretArn + ":username::"),
							},
							{
								Name:      aws.String("PROMETHEUS_PASS"),
								ValueFrom: aws.String(t.base.PrometheusSecretArn + ":password::"),
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
								"awslogs-group":         aws.String(t.base.LogGroupName),
								"awslogs-region":        aws.String(t.base.AwsRegion),
								"awslogs-stream-prefix": aws.String(logStreamPrefix),
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

			return nil
		},
	}
}

func (t *Target) deregisterTaskDefinition() Task {
	return Task{
		Name:  "deregister task definition",
		Check: t.taskDefinitionIsInactive(),
		Func: func(ctx context.Context, sess *session.Session) error {
			t.mu.Lock()
			defer t.mu.Unlock()
			return deregisterEcsTaskDefinition(ctx, sess, t.taskDefinitionArn)
		},
	}
}

func (t *Target) runTask() Task {
	return Task{
		Name:  "run task",
		Check: t.taskIsRunning(),
		Func: func(ctx context.Context, sess *session.Session) error {
			svc := ecs.New(sess)
			in := &ecs.RunTaskInput{
				CapacityProviderStrategy: []*ecs.CapacityProviderStrategyItem{
					{
						Base:             aws.Int64(0),
						CapacityProvider: aws.String(t.capacityProvider),
						Weight:           aws.Int64(1),
					},
				},
				Cluster:        aws.String(t.base.EcsClusterArn),
				Count:          aws.Int64(1),
				TaskDefinition: aws.String(t.taskDefinitionFamily),
				Group:          aws.String(t.experiment),
				PlacementStrategy: []*ecs.PlacementStrategy{
					{
						Field: aws.String("instanceId"),
						Type:  aws.String("spread"),
					},
				},
				Tags: ecsTags(t.tags()),
			}

			attempts := 3
			for attempts > 0 {
				attempts--

				out, err := svc.RunTask(in)
				if err != nil {
					return fmt.Errorf("run task: %w", err)
				}

				if out == nil {
					return fmt.Errorf("no run task output found")
				}

				if len(out.Failures) > 0 {
					for _, f := range out.Failures {
						slog.Warn("run task failure", "component", t.ComponentName(), "arn", dstr(f.Arn), "detail", dstr(f.Detail), "reason", dstr(f.Reason))
					}
					if attempts > 0 {
						slog.Debug("waiting before trying again", "component", t.ComponentName())
						time.Sleep(time.Minute)
						continue
					}
					return fmt.Errorf("run task returned failures too many times")
				}

				if len(out.Tasks) != 1 {
					return fmt.Errorf("unexpected number of tasks: %d", len(out.Tasks))
				}

				return nil

			}
			return fmt.Errorf("all attempts to run task failed")
		},
	}
}

func (t *Target) stopTask() Task {
	return Task{
		Name:  "stop task",
		Check: t.taskIsStoppedOrStopping(),
		Func: func(ctx context.Context, sess *session.Session) error {
			t.mu.Lock()
			defer t.mu.Unlock()
			return stopEcsTask(ctx, sess, t.base.EcsClusterArn, t.taskArn)
		},
	}
}

func (t *Target) taskDefinitionIsActive() Check {
	return Check{
		Name:        "task definition is active",
		FailureText: "task definition is not active",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			var err error
			t.taskDefinitionArn, t.taskDefinitionRevision, err = findTaskDefinition(t.taskDefinitionFamily, sess)
			if err != nil {
				return false, err
			}

			if t.taskDefinitionArn != "" {
				slog.Debug("captured task definition details", "component", t.ComponentName(), "task_definition_arn", t.taskDefinitionArn, "task_definition_revision", t.taskDefinitionRevision)
				return true, nil
			}
			return false, nil
		},
	}
}

func (t *Target) taskDefinitionIsInactive() Check {
	return Check{
		Name:        "task definition is inactive",
		FailureText: "task definition is active",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			var err error
			t.taskDefinitionArn, t.taskDefinitionRevision, err = findTaskDefinition(t.taskDefinitionFamily, sess)
			if err != nil {
				return false, err
			}

			if t.taskDefinitionArn != "" {
				slog.Debug("captured task definition details", "component", t.ComponentName(), "task_definition_arn", t.taskDefinitionArn, "task_definition_revision", t.taskDefinitionRevision)
				return false, nil
			}
			return true, nil
		},
	}
}

func (t *Target) taskIsRunning() Check {
	return Check{
		Name:        "task is running",
		FailureText: "task is not running",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			taskArn, err := findTask(t.base.EcsClusterArn, t.taskDefinitionFamily, sess)
			if err != nil {
				return false, err
			}

			t.mu.Lock()
			t.taskArn = taskArn
			t.mu.Unlock()

			if taskArn == "" {
				return false, nil
			}

			slog.Debug("captured task details", "component", t.ComponentName(), "task_arn", taskArn)

			running, err := isTaskRunning(ctx, sess, t.base.EcsClusterArn, taskArn)
			if err != nil {
				return false, err
			}

			if !running {
				return false, nil
			}

			svc := ecs.New(sess)
			in := &ecs.DescribeTasksInput{
				Cluster: aws.String(t.base.EcsClusterArn),
				Tasks: []*string{
					aws.String(taskArn),
				},
			}
			out, err := svc.DescribeTasks(in)
			if err != nil {
				return false, fmt.Errorf("describe tasks: %w", err)
			}

			if len(out.Failures) > 0 {
				for _, f := range out.Failures {
					slog.Warn("desctibe tasks failure", "component", t.ComponentName(), "arn", dstr(f.Arn), "detail", dstr(f.Detail), "reason", dstr(f.Reason))
				}
				return false, fmt.Errorf("failed to describe tasks")
			}

			if len(out.Tasks) != 1 {
				return false, fmt.Errorf("unexpected number of tasks: %d", len(out.Tasks))
			}

			if out.Tasks[0] == nil {
				return false, fmt.Errorf("nil task found")
			}

			task := *out.Tasks[0]
			if task.ContainerInstanceArn == nil {
				return false, fmt.Errorf("container instance arn not found")
			}

			inci := &ecs.DescribeContainerInstancesInput{
				Cluster: aws.String(t.base.EcsClusterArn),
				ContainerInstances: []*string{
					task.ContainerInstanceArn,
				},
			}
			outci, err := svc.DescribeContainerInstances(inci)
			if err != nil {
				return false, fmt.Errorf("describe container instances: %w", err)
			}

			if len(outci.Failures) > 0 {
				for _, f := range out.Failures {
					slog.Warn("desctibe container instances", "component", t.ComponentName(), "arn", dstr(f.Arn), "detail", dstr(f.Detail), "reason", dstr(f.Reason))
				}
				return false, fmt.Errorf("failed to describe container instances")
			}

			if len(outci.ContainerInstances) != 1 {
				return false, fmt.Errorf("unexpected number of container instances: %d", len(outci.ContainerInstances))
			}

			if outci.ContainerInstances[0] == nil || outci.ContainerInstances[0].Ec2InstanceId == nil {
				return false, fmt.Errorf("container instance id not found")
			}

			ec2svc := ec2.New(sess)
			ini := &ec2.DescribeInstancesInput{
				InstanceIds: []*string{
					outci.ContainerInstances[0].Ec2InstanceId,
				},
			}
			outi, err := ec2svc.DescribeInstances(ini)
			if err != nil {
				return false, fmt.Errorf("describe ec2 instances: %w", err)
			}

			if len(outi.Reservations) != 1 {
				return false, fmt.Errorf("unexpected number of instance reservations found: %d", len(outi.Reservations))
			}

			if len(outi.Reservations[0].Instances) != 1 {
				return false, fmt.Errorf("unexpected number of instances found: %d", len(outi.Reservations[0].Instances))
			}

			instance := outi.Reservations[0].Instances[0]

			if instance == nil || instance.PrivateIpAddress == nil {
				return false, fmt.Errorf("private ip address not found")
			}

			t.mu.Lock()
			defer t.mu.Unlock()
			t.taskEC2InstanceID = *outci.ContainerInstances[0].Ec2InstanceId
			t.taskPrivateIPAddress = *instance.PrivateIpAddress
			slog.Debug("captured instance details", "component", t.ComponentName(), "ec2_instance_id", *outci.ContainerInstances[0].Ec2InstanceId, "private_ip_address", *instance.PrivateIpAddress)
			return true, nil
		},
	}
}

func (t *Target) taskIsStoppedOrStopping() Check {
	return Check{
		Name:        "task is stopped or stopping",
		FailureText: "task is not stopped or stopping",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			taskArn, err := findTask(t.base.EcsClusterArn, t.taskDefinitionFamily, sess)
			if err != nil {
				return false, err
			}
			t.mu.Lock()
			t.taskArn = taskArn
			t.taskEC2InstanceID = ""
			t.taskPrivateIPAddress = ""
			t.mu.Unlock()

			if taskArn == "" {
				return true, nil
			}
			slog.Debug("captured task details", "component", t.ComponentName(), "task_arn", taskArn)

			running, err := isTaskRunning(ctx, sess, t.base.EcsClusterArn, taskArn)
			if err != nil {
				return false, err
			}

			if running {
				return false, nil
			}
			return true, nil
		},
	}
}
