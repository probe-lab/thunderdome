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
	taskRoleName                          string

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
		taskRoleName:  experiment,
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

func (b *BaseInfra) TaskRoleName() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.taskRoleName
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

	if err := b.InspectExisting(ctx); err != nil {
		return fmt.Errorf("inspect existing infra: %w", err)
	}

	return TaskSequence(ctx, sess, "base infra",
		b.createTaskRole(),
		b.attachSsmExecPolicy(),
	)

	return nil
}

func (b *BaseInfra) InspectExisting(ctx context.Context) error {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(b.awsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

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

	if b.serviceDiscoveryPrivateDnsNamespaceID == "" {
		return fmt.Errorf("did not find service discovery private DNS namespace ID")
	}

	return nil
}

func (b *BaseInfra) findExistingTaskRole(sess *session.Session) error {
	svc := iam.New(sess)
	in := &iam.GetRoleInput{
		RoleName: aws.String(b.experiment),
	}
	out, err := svc.GetRole(in)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeNoSuchEntityException:
				return nil
			default:
				return fmt.Errorf("get role: %w", err)
			}
		}

		return fmt.Errorf("get role: %w", err)
	}

	if out == nil || out.Role == nil || out.Role.Arn == nil {
		return fmt.Errorf("no arn returned")
	}

	b.taskRoleArn = *out.Role.Arn

	return nil
}

func (b *BaseInfra) Teardown(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(b.awsRegion),
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	log.Printf("base infra: starting tear down")
	if err := b.InspectExisting(ctx); err != nil {
		return fmt.Errorf("inspect existing infra: %w", err)
	}

	return TaskSequence(ctx, sess, "base infra",
		b.detachSsmExecPolicy(),
		b.deleteTaskRole(),
	)
}

func (b *BaseInfra) createTaskRole() Task {
	return Task{
		Name:  "create task role " + b.taskRoleName,
		Check: b.taskRoleExists(),
		Func: func(ctx context.Context, sess *session.Session) error {
			if err := b.findExistingTaskRole(sess); err != nil {
				return fmt.Errorf("find existing task role: %w", err)
			}

			if b.taskRoleArn != "" {
				// TODO: don't assume this is configured how we want it to be
				log.Printf("task role %q: already exists with arn %s", b.taskRoleName, b.taskRoleArn)
				return nil
			}

			svc := iam.New(sess)
			in := &iam.CreateRoleInput{
				RoleName: aws.String(b.taskRoleName),
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
			log.Printf("task role %q: creating", b.taskRoleName)
			out, err := svc.CreateRole(in)
			if err != nil {
				return fmt.Errorf("create role: %w", err)
			}

			if out == nil || out.Role == nil || out.Role.Arn == nil {
				return fmt.Errorf("no arn returned")
			}

			b.taskRoleArn = *out.Role.Arn
			log.Printf("task role %q: created with arn %s", b.taskRoleName, b.taskRoleArn)
			return nil
		},
	}
}

func (b *BaseInfra) deleteTaskRole() Task {
	return Task{
		Name:  "delete task role " + b.taskRoleName,
		Check: b.taskRoleDoesNotExist(),
		Func: func(ctx context.Context, sess *session.Session) error {
			svc := iam.New(sess)

			in := &iam.DeleteRoleInput{
				RoleName: aws.String(b.taskRoleName),
			}

			_, err := svc.DeleteRole(in)
			if err != nil {
				if iamIsNoSuchEntity(err) {
					return nil
				}
				return fmt.Errorf("delete role: %w", err)
			}
			return nil
		},
	}
}

func (b *BaseInfra) attachSsmExecPolicy() Task {
	return Task{
		Name:  "attach ssm exec policy to role " + b.taskRoleName,
		Check: b.ssmExecPolicyAttached(),
		Func: func(ctx context.Context, sess *session.Session) error {
			svc := iam.New(sess)

			in := &iam.AttachRolePolicyInput{
				RoleName:  aws.String(b.taskRoleName),
				PolicyArn: aws.String(ssm_exec_policy_arn),
			}

			if _, err := svc.AttachRolePolicy(in); err != nil {
				return fmt.Errorf("attach role policy: %w", err)
			}

			return nil
		},
	}
}

func (b *BaseInfra) detachSsmExecPolicy() Task {
	return Task{
		Name:  "detach ssm exec policy from role " + b.taskRoleName,
		Check: b.ssmExecPolicyDetached(),
		Func: func(ctx context.Context, sess *session.Session) error {
			svc := iam.New(sess)

			in := &iam.DetachRolePolicyInput{
				RoleName:  aws.String(b.taskRoleName),
				PolicyArn: aws.String(ssm_exec_policy_arn),
			}

			if _, err := svc.DetachRolePolicy(in); err != nil {
				if iamIsNoSuchEntity(err) {
					return nil
				}
				return fmt.Errorf("detach role policy: %w", err)
			}
			return nil
		},
	}
}

func (b *BaseInfra) Ready(ctx context.Context) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(b.awsRegion),
	})
	if err != nil {
		return false, fmt.Errorf("new session: %w", err)
	}

	if err := b.InspectExisting(ctx); err != nil {
		return false, fmt.Errorf("inspect existing infra: %w", err)
	}

	return CheckSequence(ctx, sess, "base infra",
		b.taskRoleExists(),
		b.ssmExecPolicyAttached(),
	)
}

func (b *BaseInfra) taskRoleDoesNotExist() Check {
	return Check{
		Name:        "task role does not exist",
		FailureText: "task role exists",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			svc := iam.New(sess)
			in := &iam.GetRoleInput{
				RoleName: aws.String(b.experiment),
			}
			out, err := svc.GetRole(in)
			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					switch aerr.Code() {
					case iam.ErrCodeNoSuchEntityException:
						return true, nil // ready check failed
					default:
						return false, fmt.Errorf("get role: %w", err)
					}
				}

				return false, fmt.Errorf("get role: %w", err)
			}

			if out == nil || out.Role == nil {
				return false, fmt.Errorf("role not found")
			}

			return false, nil
		},
	}
}

func (b *BaseInfra) taskRoleExists() Check {
	return Check{
		Name:        "task role exists",
		FailureText: "task role does not exist",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			svc := iam.New(sess)
			in := &iam.GetRoleInput{
				RoleName: aws.String(b.experiment),
			}
			out, err := svc.GetRole(in)
			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					switch aerr.Code() {
					case iam.ErrCodeNoSuchEntityException:
						return false, nil
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
		},
	}
}

func (b *BaseInfra) ssmExecPolicyAttached() Check {
	return Check{
		Name: "ssm exec policy attached",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
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
				return false, nil
			}

			for _, p := range out.AttachedPolicies {
				if p.PolicyArn != nil && *p.PolicyArn == ssm_exec_policy_arn {
					return true, nil
				}
			}

			if out.IsTruncated != nil && *out.IsTruncated == true {
				// TODO: implement fetching next page of results
				return false, fmt.Errorf("policy not found but there are results that were not fetched (unimplemented behaviour)")
			}
			return false, nil
		},
	}
}

func (b *BaseInfra) ssmExecPolicyDetached() Check {
	return Check{
		Name: "ssm exec policy detached",
		Func: func(ctx context.Context, sess *session.Session) (bool, error) {
			svc := iam.New(sess)
			in := &iam.ListAttachedRolePoliciesInput{
				RoleName: aws.String(b.experiment),
			}
			out, err := svc.ListAttachedRolePolicies(in)
			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					switch aerr.Code() {
					case iam.ErrCodeNoSuchEntityException:
						return true, nil
					default:
						return false, err
					}
				}

				return false, err
			}

			if out == nil || out.AttachedPolicies == nil {
				return true, nil
			}

			for _, p := range out.AttachedPolicies {
				if p.PolicyArn != nil && *p.PolicyArn == ssm_exec_policy_arn {
					return false, nil
				}
			}

			if out.IsTruncated != nil && *out.IsTruncated == true {
				// TODO: implement fetching next page of results
				return false, fmt.Errorf("policy not found but there are results that were not fetched (unimplemented behaviour)")
			}
			return true, nil
		},
	}
}

func iamIsNoSuchEntity(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		return aerr.Code() == iam.ErrCodeNoSuchEntityException
	}
	return false
}
