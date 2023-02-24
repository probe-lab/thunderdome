package infra

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"golang.org/x/exp/slog"

	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/api"
)

type Dealgood struct {
	experiment  string
	base        *BaseInfra
	image       string
	environment map[string]string

	taskDefinitionFamily   string
	taskName               string
	requestQueueName       string
	requestQueuePolicyPath string

	// mu guards access to fields in block directly below
	mu                     sync.Mutex
	ready                  bool
	taskDefinitionArn      string
	taskDefinitionRevision int64
	taskArn                string
	requestQueueArn        string
	requestQueueURL        string
	requestSubscriptionArn string
}

func NewDealgood(experiment string, base *BaseInfra) *Dealgood {
	requestQueueName := experiment + "-dealgood-requests"

	env := map[string]string{
		"DEALGOOD_EXPERIMENT":         experiment,
		"OTEL_TRACES_EXPORTER":        "otlp",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317",
		"DEALGOOD_RATE":               "10",
		"DEALGOOD_FILTER":             "pathonly",
		"DEALGOOD_CONCURRENCY":        "100",
		"DEALGOOD_LOKI_URI":           "https://logs-prod-us-central1.grafana.net",
		"DEALGOOD_LOKI_QUERY":         "{job=\"nginx\",app=\"gateway\",team=\"bifrost\"}",
		"DEALGOOD_SQS_REGION":         base.AwsRegion,
		"DEALGOOD_SOURCE":             "sqs",
		"DEALGOOD_SQS_QUEUE":          requestQueueName,
		"DEALGOOD_PRE_PROBE_WAIT":     "0",
	}

	return &Dealgood{
		experiment:           experiment,
		base:                 base,
		image:                base.DealgoodImage,
		environment:          env,
		taskDefinitionFamily: experiment + "-dealgood",
		taskName:             experiment + "-dealgood",
		requestQueueName:     requestQueueName,
	}
}

func (d *Dealgood) WithMaxRequestRate(v int) *Dealgood {
	d.environment["DEALGOOD_RATE"] = strconv.Itoa(v)
	return d
}

func (d *Dealgood) WithMaxConcurrency(v int) *Dealgood {
	d.environment["DEALGOOD_CONCURRENCY"] = strconv.Itoa(v)
	return d
}

func (d *Dealgood) WithRequestFilter(v string) *Dealgood {
	d.environment["DEALGOOD_FILTER"] = v
	return d
}

func (d *Dealgood) WithTargets(targets []*Target) *Dealgood {
	targetURLs := make([]string, len(targets))
	for i := range targets {
		targetURLs[i] = targets[i].Name() + "::" + targets[i].GatewayURL()
	}

	d.environment["DEALGOOD_TARGETS"] = strings.Join(targetURLs, ",")
	return d
}

func (d *Dealgood) Name() string {
	return "dealgood"
}

func (d *Dealgood) ComponentName() string {
	return "dealgood"
}

func (d *Dealgood) TaskDefinitionArn() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.ready {
		return ""
	}
	return d.taskDefinitionArn
}

func (d *Dealgood) TaskArn() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.ready {
		return ""
	}
	return d.taskArn
}

func (d *Dealgood) Resources() []api.Resource {
	d.mu.Lock()
	defer d.mu.Unlock()

	var res []api.Resource
	res = append(res, api.Resource{
		Type: api.ResourceTypeEcsTask,
		Keys: map[string]string{
			api.ResourceKeyEcsClusterArn: d.base.EcsClusterArn,
			api.ResourceKeyArn:           d.taskArn,
		},
	})
	res = append(res, api.Resource{
		Type: api.ResourceTypeEcsTaskDefinition,
		Keys: map[string]string{
			api.ResourceKeyArn: d.taskDefinitionArn,
		},
	})
	res = append(res, api.Resource{
		Type: api.ResourceTypeEcsSnsSubscription,
		Keys: map[string]string{
			api.ResourceKeyArn: d.requestSubscriptionArn,
		},
	})
	res = append(res, api.Resource{
		Type: api.ResourceTypeSqsQueue,
		Keys: map[string]string{
			api.ResourceKeyArn:      d.requestQueueArn,
			api.ResourceKeyQueueURL: d.requestQueueURL,
		},
	})
	return res
}

func (d *Dealgood) Setup(ctx context.Context) error {
	slog.Info("starting setup", "component", d.Name())
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(d.base.AwsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	return TaskSequence(ctx, sess, d.Name(),
		d.createRequestQueue(),
		d.createRequestQueueSubscription(),
		d.createTaskDefinition(),
		d.runTask(),
	)
}

func (d *Dealgood) Teardown(ctx context.Context) error {
	slog.Info("starting teardown", "component", d.Name())
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(d.base.AwsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	return TaskSequence(ctx, sess, d.Name(),
		d.stopTask(),
		d.deregisterTaskDefinition(),
		d.deleteRequestQueueSubscription(),
		d.deleteRequestQueue(),
	)
}

func (d *Dealgood) Ready(ctx context.Context) (bool, error) {
	d.mu.Lock()
	d.ready = false
	d.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(d.base.AwsRegion),
	})
	if err != nil {
		return false, fmt.Errorf("new session: %w", err)
	}

	ready, err := CheckSequence(ctx, sess, d.Name(),
		d.requestQueueExists(),
		d.requestQueueSubscriptionExists(),
		d.taskDefinitionIsActive(),
		d.taskIsRunning(),
	)

	if !ready || err != nil {
		return ready, err
	}

	d.mu.Lock()
	d.ready = true
	d.mu.Unlock()
	return true, nil
}

func (d *Dealgood) tags() map[string]*string {
	return map[string]*string{
		"experiment": aws.String(d.experiment),
		"component":  aws.String(d.Name()),
	}
}

func (d *Dealgood) createTaskDefinition() Task {
	return Task{
		Name:  "create task definition",
		Check: d.taskDefinitionIsActive(),
		Func: func(ctx context.Context, sess *session.Session) error {
			in := &ecs.RegisterTaskDefinitionInput{
				Family:                  aws.String(d.taskDefinitionFamily),
				RequiresCompatibilities: []*string{aws.String("FARGATE")},
				NetworkMode:             aws.String("awsvpc"),
				ExecutionRoleArn:        aws.String(d.base.EcsExecutionRoleArn),
				TaskRoleArn:             aws.String(d.base.DealgoodTaskRoleArn),
				Cpu:                     aws.String("4096"),
				Memory:                  aws.String("10240"),
				Tags:                    ecsTags(d.tags()),
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
							FileSystemId: aws.String(d.base.EfsFileSystemID),
						},
					},
				},
				ContainerDefinitions: []*ecs.ContainerDefinition{
					{
						Name:        aws.String("dealgood"),
						Image:       aws.String(d.image),
						Cpu:         aws.Int64(0),
						Essential:   aws.Bool(true),
						Environment: mapsToKeyValuePair(d.environment),

						Secrets: []*ecs.Secret{},
						MountPoints: []*ecs.MountPoint{
							{
								SourceVolume:  aws.String("efs"),
								ContainerPath: aws.String("/mnt/efs"),
							},
						},
						LogConfiguration: &ecs.LogConfiguration{
							LogDriver: aws.String("awslogs"),
							Options: map[string]*string{
								"awslogs-group":         aws.String(d.base.LogGroupName),
								"awslogs-region":        aws.String(d.base.AwsRegion),
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
							aws.String("-config.file=" + d.base.DealgoodGrafanaAgentConfigURL),
						},
						Environment: []*ecs.KeyValuePair{
							// we use these for setting labels on metrics
							{
								Name:  aws.String("THUNDERDOME_EXPERIMENT"),
								Value: aws.String(d.experiment),
							},
						},
						Essential: aws.Bool(true),
						LogConfiguration: &ecs.LogConfiguration{
							LogDriver: aws.String("awslogs"),
							Options: map[string]*string{
								"awslogs-group":         aws.String(d.base.LogGroupName),
								"awslogs-region":        aws.String(d.base.AwsRegion),
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
								ValueFrom: aws.String(d.base.GrafanaPushSecretArn + ":username::"),
							},
							{
								Name:      aws.String("GRAFANA_PASS"),
								ValueFrom: aws.String(d.base.GrafanaPushSecretArn + ":password::"),
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

func (d *Dealgood) deregisterTaskDefinition() Task {
	return Task{
		Name:  "deregister task definition",
		Check: d.taskDefinitionIsInactive(),
		Func: func(ctx context.Context, sess *session.Session) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			return deregisterEcsTaskDefinition(ctx, sess, d.taskDefinitionArn)
		},
	}
}

func (d *Dealgood) runTask() Task {
	return Task{
		Name:  "run task",
		Check: d.taskIsRunning(),
		Func: func(ctx context.Context, sess *session.Session) error {
			svc := ecs.New(sess)

			in := &ecs.RunTaskInput{
				LaunchType: aws.String("FARGATE"),
				NetworkConfiguration: &ecs.NetworkConfiguration{
					AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
						AssignPublicIp: aws.String("ENABLED"),
						SecurityGroups: []*string{
							aws.String(d.base.DealgoodSecurityGroup),
						},
						Subnets: []*string{
							aws.String(d.base.VpcPublicSubnet),
						},
					},
				},
				Cluster:        aws.String(d.base.EcsClusterArn),
				Count:          aws.Int64(1),
				TaskDefinition: aws.String(d.taskDefinitionFamily),
				Tags:           ecsTags(d.tags()),
			}

			out, err := svc.RunTask(in)
			if err != nil {
				return fmt.Errorf("run task: %w", err)
			}

			if out == nil {
				return fmt.Errorf("no run task output found")
			}

			if len(out.Failures) > 0 {
				for _, f := range out.Failures {
					slog.Warn("run task failure", "component", d.Name(), "arn", dstr(f.Arn), "detail", dstr(f.Detail), "reason", dstr(f.Reason))
				}
			}

			if len(out.Tasks) != 1 {
				return fmt.Errorf("run task returned unexpected number of tasks: %d", len(out.Tasks))
			}

			return nil
		},
	}
}

func (d *Dealgood) stopTask() Task {
	return Task{
		Name:  "stop task",
		Check: d.taskIsStoppedOrStopping(),
		Func: func(ctx context.Context, sess *session.Session) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			return stopEcsTask(ctx, sess, d.base.EcsClusterArn, d.taskArn)
		},
	}
}

func (d *Dealgood) createRequestQueue() Task {
	return Task{
		Name:  "create request queue",
		Check: d.requestQueueExists(),
		Func: func(ctx context.Context, sess *session.Session) error {
			svc := sqs.New(sess)

			in := &sqs.CreateQueueInput{
				QueueName: aws.String(d.requestQueueName),
				Tags:      d.tags(),
			}

			var err error
			out, err := svc.CreateQueue(in)
			if err != nil {
				if sqsIsQueueDeletedRecently(err) {
					slog.Info("queue was deleted recently, waiting 60 seconds to recreate", "component", d.Name())
					time.Sleep(60 * time.Second)
					out, err = svc.CreateQueue(in)
				}
			}
			if err != nil {
				return fmt.Errorf("create queue: %w", err)
			}

			if out == nil || out.QueueUrl == nil {
				return fmt.Errorf("no queue created")
			}

			inga := &sqs.GetQueueAttributesInput{
				AttributeNames: []*string{
					aws.String("QueueArn"),
				},
				QueueUrl: out.QueueUrl,
			}
			outga, err := svc.GetQueueAttributes(inga)
			if err != nil {
				return fmt.Errorf("get queue attributes: %w", err)
			}

			if outga == nil || len(outga.Attributes) == 0 {
				return fmt.Errorf("no queue attributes found")
			}

			queueArn, ok := outga.Attributes["QueueArn"]
			if !ok || queueArn == nil {
				return fmt.Errorf("no queue arn found")
			}

			policy := fmt.Sprintf(`{
				  "Version": "2012-10-17",
				  "Id": "sqspolicy",
				  "Statement": [
				    {
				      "Sid": "First",
				      "Effect": "Allow",
				      "Action": "sqs:SendMessage",
				      "Principal": "*",
				      "Resource": "%s",
				      "Condition": {
				        "ArnEquals": {
				          "aws:SourceArn": "%s"
				        }
				      }
				    }
				  ]
				}`, *queueArn, d.base.RequestSNSTopicArn)

			insa := &sqs.SetQueueAttributesInput{
				Attributes: map[string]*string{
					"Policy": aws.String(policy),
				},
				QueueUrl: out.QueueUrl,
			}

			if _, err := svc.SetQueueAttributes(insa); err != nil {
				return fmt.Errorf("set queue attributes: %w", err)
			}

			return nil
		},
	}
}

func (d *Dealgood) deleteRequestQueue() Task {
	return Task{
		Name:  "delete request queue",
		Check: d.requestQueueDoesNotExist(),
		Func: func(ctx context.Context, sess *session.Session) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			return deleteSqsQueue(ctx, sess, d.requestQueueURL)
		},
	}
}

func (d *Dealgood) createRequestQueueSubscription() Task {
	return Task{
		Name:  "create request queue subscription",
		Check: d.requestQueueSubscriptionExists(),
		Func: func(ctx context.Context, sess *session.Session) error {
			// arn is outpol.Policy.Arn

			// # Subscribe queue to requests topic
			// resource "aws_sns_topic_subscription" "requests_sqs_target" {
			//   topic_arn = var.request_sns_topic_arn
			//   protocol  = "sqs"
			//   endpoint  = aws_sqs_queue.requests.arn
			// }

			snssvc := sns.New(sess)

			insub := &sns.SubscribeInput{
				TopicArn: aws.String(d.base.RequestSNSTopicArn),
				Protocol: aws.String("sqs"),
				Endpoint: aws.String(d.requestQueueArn),
			}

			outsub, err := snssvc.Subscribe(insub)
			if err != nil {
				return fmt.Errorf("subscribe to request topic: %w", err)
			}

			if outsub == nil || outsub.SubscriptionArn == nil {
				return fmt.Errorf("no subscription arn found")
			}

			return nil
		},
	}
}

func (d *Dealgood) deleteRequestQueueSubscription() Task {
	return Task{
		Name:  "delete request queue subscription",
		Check: d.requestQueueSubscriptionDoesNotExist(),
		Func: func(ctx context.Context, sess *session.Session) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			return unsubscribeSqsQueue(ctx, sess, d.requestSubscriptionArn)
		},
	}
}

func (d *Dealgood) taskDefinitionIsActive() Check {
	return Check{
		Name:        "task definition is active",
		FailureText: "task definition is not active",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			var err error
			d.taskDefinitionArn, d.taskDefinitionRevision, err = findTaskDefinition(d.taskDefinitionFamily, sess)
			if err != nil {
				return false, err
			}

			if d.taskDefinitionArn != "" {
				slog.Debug("captured task definition details", "component", d.Name(), "task_definition_arn", d.taskDefinitionArn, "task_definition_revision", d.taskDefinitionRevision)
				return true, nil
			}
			return false, nil
		},
	}
}

func (d *Dealgood) taskDefinitionIsInactive() Check {
	return Check{
		Name:        "task definition is inactive",
		FailureText: "task definition is active",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			var err error
			d.taskDefinitionArn, d.taskDefinitionRevision, err = findTaskDefinition(d.taskDefinitionFamily, sess)
			if err != nil {
				return false, err
			}

			if d.taskDefinitionArn != "" {
				slog.Debug("captured task definition details", "component", d.Name(), "task_definition_arn", d.taskDefinitionArn, "task_definition_revision", d.taskDefinitionRevision)
				return false, nil
			}
			return true, nil
		},
	}
}

func (d *Dealgood) taskIsRunning() Check {
	return Check{
		Name:        "task is running",
		FailureText: "task is not running",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			taskArn, err := findTask(d.base.EcsClusterArn, d.taskDefinitionFamily, sess)
			if err != nil {
				return false, err
			}

			d.mu.Lock()
			d.taskArn = taskArn
			d.mu.Unlock()
			if taskArn == "" {
				return false, nil
			}
			slog.Debug("captured task details", "component", d.Name(), "task_arn", taskArn)

			running, err := isTaskRunning(ctx, sess, d.base.EcsClusterArn, taskArn)
			if err != nil {
				return false, err
			}

			if running {
				return true, nil
			}
			return false, nil
		},
	}
}

func (d *Dealgood) taskIsStoppedOrStopping() Check {
	return Check{
		Name:        "task is stopped or stopping",
		FailureText: "task is not stopped or stopping",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			taskArn, err := findTask(d.base.EcsClusterArn, d.taskDefinitionFamily, sess)
			if err != nil {
				return false, err
			}
			d.mu.Lock()
			d.taskArn = taskArn
			d.mu.Unlock()

			if taskArn == "" {
				return true, nil
			}
			slog.Debug("captured task details", "component", d.Name(), "task_arn", taskArn)

			running, err := isTaskRunning(ctx, sess, d.base.EcsClusterArn, taskArn)
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

func (d *Dealgood) requestQueueExists() Check {
	return Check{
		Name:        "request queue exists",
		FailureText: "request queue does not exist",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			queueArn, queueURL, err := findQueue(d.requestQueueName, sess)
			if err != nil {
				return false, err
			}

			d.mu.Lock()
			defer d.mu.Unlock()
			d.requestQueueArn = queueArn
			d.requestQueueURL = queueURL

			if d.requestQueueArn != "" {
				slog.Debug("captured request queue details", "component", d.Name(), "request_queue_arn", d.requestQueueArn, "request_queue_url", d.requestQueueURL)
				return true, nil
			}
			return false, nil
		},
	}
}

func (d *Dealgood) requestQueueDoesNotExist() Check {
	return Check{
		Name:        "request queue does not exist",
		FailureText: "request queue exists",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			queueArn, queueURL, err := findQueue(d.requestQueueName, sess)
			if err != nil {
				return false, err
			}

			d.mu.Lock()
			defer d.mu.Unlock()
			d.requestQueueArn = queueArn
			d.requestQueueURL = queueURL

			if d.requestQueueArn != "" {
				slog.Debug("captured request queue details", "component", d.Name(), "request_queue_arn", d.requestQueueArn, "request_queue_url", d.requestQueueURL)
				return false, nil
			}
			return true, nil
		},
	}
}

func (d *Dealgood) requestQueueSubscriptionExists() Check {
	return Check{
		Name:        "request queue subscription exists",
		FailureText: "request queue subscription does not exist",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			d.mu.Lock()
			requestQueueArn := d.requestQueueArn
			d.mu.Unlock()

			subscriptionArn, err := findSubscription(d.base.RequestSNSTopicArn, requestQueueArn, sess)
			if err != nil {
				return false, err
			}

			d.mu.Lock()
			defer d.mu.Unlock()
			d.requestSubscriptionArn = subscriptionArn

			if d.requestSubscriptionArn != "" {
				slog.Debug("request queue subscription exists", "component", d.Name(), "request_queue_arn", requestQueueArn, "request_subscription_arn", d.requestSubscriptionArn)
				return true, nil
			}
			slog.Debug("request queue subscription does not exist", "component", d.Name())
			return false, nil
		},
	}
}

func (d *Dealgood) requestQueueSubscriptionDoesNotExist() Check {
	return Check{
		Name:        "request queue subscription does not exist",
		FailureText: "request queue subscription exists",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			d.mu.Lock()
			requestQueueArn := d.requestQueueArn
			d.mu.Unlock()

			subscriptionArn, err := findSubscription(d.base.RequestSNSTopicArn, requestQueueArn, sess)
			if err != nil {
				return false, err
			}

			d.mu.Lock()
			defer d.mu.Unlock()
			d.requestSubscriptionArn = subscriptionArn

			if d.requestSubscriptionArn != "" {
				slog.Debug("request queue subscription exists", "component", d.Name(), "request_queue_arn", requestQueueArn, "request_subscription_arn", d.requestSubscriptionArn)
				return false, nil
			}
			slog.Debug("request queue subscription does not exist", "component", d.Name())
			return true, nil
		},
	}
}
