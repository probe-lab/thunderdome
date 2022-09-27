package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
)

// TODO: look these up
const (
	ssm_exec_policy_arn             = "arn:aws:iam::147263665150:policy/ssm-exec"
	execution_role_arn              = "arn:aws:iam::147263665150:role/ecsTaskExecutionRole"
	efs_file_system_id              = "fs-006bd3d793700a2df"
	grafana_agent_target_config_url = "https://pl-thunderdome-public.s3.eu-west-1.amazonaws.com/grafana-agent-config/target.yaml"
	grafana_push_secret_arn         = "arn:aws:secretsmanager:eu-west-1:147263665150:secret:grafana-push-MxjNiv"
)

type BaseInfra struct {
	experiment                            string
	awsRegion                             string
	logGroupName                          string
	serviceDiscoveryPrivateDnsNamespaceID string
	ecsClusterArn                         string

	mu          sync.Mutex
	ready       bool
	taskRoleArn string
}

func NewBaseInfra(experiment, awsRegion string, ecsClusterArn string) *BaseInfra {
	return &BaseInfra{
		experiment:    experiment,
		awsRegion:     awsRegion,
		ecsClusterArn: ecsClusterArn,
		logGroupName:  "thunderdome", // aws_cloudwatch_log_group.logs.name
	}
}

func (b *BaseInfra) TaskRoleArn() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.taskRoleArn
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

func (b *BaseInfra) ServiceDiscoveryPrivateDnsNamespaceID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.serviceDiscoveryPrivateDnsNamespaceID
}

func (b *BaseInfra) Setup(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(b.awsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	if err := b.inspectExisting(ctx, sess); err != nil {
		return fmt.Errorf("inspect existing infra: %w", err)
	}

	if err := b.setupTaskRole(sess); err != nil {
		return fmt.Errorf("setup task role: %w", err)
	}

	if err := b.setupTaskRolePolicyAttachments(sess); err != nil {
		return fmt.Errorf("setup task role policy attachments: %w", err)
	}

	return nil
}

func (b *BaseInfra) inspectExisting(ctx context.Context, sess *session.Session) error {
	svc := servicediscovery.New(sess)

	in := &servicediscovery.ListNamespacesInput{
		Filters: []*servicediscovery.NamespaceFilter{
			{
				Condition: aws.String("EQ"),
				Name:      aws.String("TYPE"),
				Values: []*string{
					aws.String("DNS_PRIVATE"),
				},
			},
		},
	}

	out, err := svc.ListNamespaces(in)
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	for _, ns := range out.Namespaces {
		if ns != nil && ns.Name != nil && *ns.Name == "thunder.dome" {
			b.serviceDiscoveryPrivateDnsNamespaceID = *ns.Id
			break
		}
	}

	return nil
}

func (b *BaseInfra) setupTaskRole(sess *session.Session) error {
	svc := iam.New(sess)
	// Create Role
	in := &iam.CreateRoleInput{
		RoleName: aws.String(b.experiment),
		AssumeRolePolicyDocument: aws.String(`{
		    "Version":"2012-10-17",
		    "Statement": [
		      {
		        "Action": "sts:AssumeRole",
		        "Effect": "Allow",
		        "Sid": "",
		        "Principal": {
		          "Service": "ecs-tasks.amazonaws.com"
		        }
		      }
		    ]
		  }`),
	}
	out, err := svc.CreateRole(in)
	if err != nil {
		return fmt.Errorf("create role: %w", err)
	}

	if out == nil || out.Role == nil || out.Role.Arn == nil {
		return fmt.Errorf("no arn returned")
	}

	b.taskRoleArn = *out.Role.Arn
	return nil
}

func (b *BaseInfra) setupTaskRolePolicyAttachments(sess *session.Session) error {
	svc := iam.New(sess)

	// Create Role
	in := &iam.AttachRolePolicyInput{
		RoleName:  aws.String(b.experiment),
		PolicyArn: aws.String(ssm_exec_policy_arn),
	}

	if _, err := svc.AttachRolePolicy(in); err != nil {
		return fmt.Errorf("attach role policy: %w", err)
	}

	return nil
}

func (b *BaseInfra) Teardown(context.Context) error {
	panic("not implemented")
}

func (b *BaseInfra) Ready(ctx context.Context) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ready {
		return true, nil
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(b.awsRegion),
	})
	if err != nil {
		return false, fmt.Errorf("new session: %w", err)
	}

	log.Printf("checking if task role is ready")
	taskRoleReady, err := b.readyTaskRole(ctx, sess)
	if err != nil {
		return false, fmt.Errorf("task role: %w", err)
	}
	if !taskRoleReady {
		return false, nil
	}

	log.Printf("checking if task role policy attachment is ready")
	taskRolePolicyAttachmentsReady, err := b.readyTaskRolePolicyAttachments(ctx, sess)
	if err != nil {
		return false, fmt.Errorf("task role policy attachments: %w", err)
	}
	if !taskRolePolicyAttachmentsReady {
		return false, nil
	}

	return true, nil
}

func (b *BaseInfra) readyTaskRole(ctx context.Context, sess *session.Session) (bool, error) {
	svc := iam.New(sess)
	in := &iam.GetRoleInput{
		RoleName: aws.String(b.experiment),
	}
	out, err := svc.GetRole(in)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeNoSuchEntityException:
				return false, nil // ready check failed
			default:
				return false, fmt.Errorf("get role: %w", err)
			}
		}

		return false, fmt.Errorf("get role: %w", err)
	}

	if out == nil || out.Role == nil {
		return false, fmt.Errorf("role not found")
	}

	return true, nil
}

func (b *BaseInfra) readyTaskRolePolicyAttachments(ctx context.Context, sess *session.Session) (bool, error) {
	svc := iam.New(sess)
	in := &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(b.experiment),
	}
	out, err := svc.ListAttachedRolePolicies(in)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeNoSuchEntityException:
				return false, nil // ready check failed
			default:
				return false, err
			}
		}

		return false, err
	}

	if out == nil || out.AttachedPolicies == nil {
		return false, fmt.Errorf("no attached policies found")
	}

	found := false
	for _, p := range out.AttachedPolicies {
		if p.PolicyArn != nil && *p.PolicyArn == ssm_exec_policy_arn {
			found = true
			break
		}
	}

	if !found {
		if out.IsTruncated != nil && *out.IsTruncated == true {
			// TODO: implement fetching next page of results
			return false, fmt.Errorf("policy not found but there are results that were not fetched (unimplemented behaviour)")
		}
		return false, nil
	}

	return true, nil
}

func (b *BaseInfra) Status(ctx context.Context) ([]ComponentStatus, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(b.awsRegion),
	})
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	statusFuncs := []struct {
		Name string
		Func func(context.Context, *session.Session) (bool, error)
	}{
		{
			Name: "task role",
			Func: b.readyTaskRole,
		},
		{
			Name: "task role policy attachment",
			Func: b.readyTaskRolePolicyAttachments,
		},
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
