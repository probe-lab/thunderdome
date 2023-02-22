package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"golang.org/x/exp/slog"
)

func isTaskActive(ctx context.Context, sess *session.Session, ecsClusterArn, taskArn string) (bool, error) {
	logger := slog.With("arn", taskArn, "cluster_arn", ecsClusterArn)
	logger.Debug("checking if task is active")
	svc := ecs.New(sess)
	in := &ecs.DescribeTasksInput{
		Cluster: aws.String(ecsClusterArn),
		Tasks: []*string{
			aws.String(taskArn),
		},
	}

	out, err := svc.DescribeTasks(in)
	if err != nil {
		return true, fmt.Errorf("describe tasks: %w", err)
	}
	for _, ta := range out.Tasks {
		if ta.TaskArn != nil && *ta.TaskArn == taskArn {
			if ta.LastStatus != nil {
				logger.Debug("task found", "status", *ta.LastStatus)
				if *ta.LastStatus == "DEACTIVATING" ||
					*ta.LastStatus == "STOPPING" ||
					*ta.LastStatus == "DEPROVISIONING" ||
					*ta.LastStatus == "STOPPED" ||
					*ta.LastStatus == "DELETED" {
					return false, nil
				}
				return true, nil
			}

			logger.Warn("task found, but cannot read status")
			return true, nil
		}
	}
	logger.Debug("task not found")
	return false, nil
}

func isTaskDefinitionActive(ctx context.Context, sess *session.Session, arn string) (bool, error) {
	logger := slog.With("arn", arn)
	logger.Debug("checking if task definition is active")

	svc := ecs.New(sess)
	in := &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: aws.String(arn),
	}

	out, err := svc.DescribeTaskDefinition(in)
	if err != nil {
		return true, fmt.Errorf("describe task definition: %w", err)
	}

	if out.TaskDefinition != nil && out.TaskDefinition.TaskDefinitionArn != nil && *out.TaskDefinition.TaskDefinitionArn == arn {
		if out.TaskDefinition.Status != nil && *out.TaskDefinition.Status == ecs.TaskDefinitionStatusActive {
			logger.Debug("task definition active")
			return true, nil
		}
		logger.Debug("task definition not active")
		return false, nil
	}

	logger.Debug("task definition not found")
	return false, nil
}

func isSnsSubscriptionActive(ctx context.Context, sess *session.Session, arn string) (bool, error) {
	logger := slog.With("arn", arn)
	logger.Debug("checking if subscription is active")

	svc := sns.New(sess)
	in := &sns.GetSubscriptionAttributesInput{
		SubscriptionArn: aws.String(arn),
	}

	out, err := svc.GetSubscriptionAttributes(in)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == sns.ErrCodeNotFoundException {
				return false, nil
			}
		}
		return true, fmt.Errorf("get subscription attributes: %w", err)
	}

	if out == nil || out.Attributes == nil {
		logger.Debug("no attributes found")
		return false, nil
	}

	if out.Attributes["TopicArn"] != nil {
		logger.Debug("topic found")
		return true, nil
	}

	logger.Debug("no topic found")
	return false, nil
}

func isSqsQueueActive(ctx context.Context, sess *session.Session, url string) (bool, error) {
	logger := slog.With("url", url)
	logger.Debug("checking if queue is active")

	svc := sqs.New(sess)
	in := &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(url),
		AttributeNames: []*string{
			aws.String("QueueArn"),
		},
	}

	out, err := svc.GetQueueAttributes(in)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == sqs.ErrCodeQueueDoesNotExist {
				return false, nil
			}
		}
		return true, fmt.Errorf("get subscription attributes: %w", err)
	}

	if out == nil || out.Attributes == nil {
		logger.Debug("no attributes found")
		return false, nil
	}

	if out.Attributes["QueueArn"] != nil {
		logger.Debug("queue arn found")
		return true, nil
	}

	logger.Debug("no queue found")
	return false, nil
}

func stopEcsTask(ctx context.Context, sess *session.Session, ecsClusterArn, taskArn string) error {
	svc := ecs.New(sess)

	in := &ecs.StopTaskInput{
		Cluster: aws.String(ecsClusterArn),
		Task:    aws.String(taskArn),
	}

	if _, err := svc.StopTask(in); err != nil {
		return fmt.Errorf("stop task: %w", err)
	}
	return nil
}

func deregisterEcsTaskDefinition(ctx context.Context, sess *session.Session, arn string) error {
	in := &ecs.DeregisterTaskDefinitionInput{
		TaskDefinition: aws.String(arn),
	}

	svc := ecs.New(sess)
	_, err := svc.DeregisterTaskDefinition(in)
	if err != nil {
		return fmt.Errorf("deregister task definition: %w", err)
	}
	return nil
}

func unsubscribeSqsQueue(ctx context.Context, sess *session.Session, arn string) error {
	snssvc := sns.New(sess)

	in := &sns.UnsubscribeInput{
		SubscriptionArn: aws.String(arn),
	}

	_, err := snssvc.Unsubscribe(in)
	if err != nil {
		return fmt.Errorf("unsubscribe from request topic: %w", err)
	}

	return nil
}

func deleteSqsQueue(ctx context.Context, sess *session.Session, queueURL string) error {
	svc := sqs.New(sess)

	in := &sqs.DeleteQueueInput{
		QueueUrl: aws.String(queueURL),
	}

	_, err := svc.DeleteQueue(in)
	if err != nil {
		return fmt.Errorf("delete queue: %w", err)
	}
	return nil
}
