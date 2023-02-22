package infra

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"golang.org/x/exp/slog"
)

func findTaskDefinition(family string, sess *session.Session) (string, int64, error) {
	svc := ecs.New(sess)
	in := &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: aws.String(family),
	}
	out, err := svc.DescribeTaskDefinition(in)
	if err != nil {
		aerr := &ecs.ClientException{}
		if errors.As(err, &aerr) {
			if strings.Contains(aerr.Error(), "Unable to describe task definition") {
				return "", 0, nil // assume it does not exist, so no error
			}
		}
		return "", 0, fmt.Errorf("describe task definition: %w", err)
	}

	if out == nil || out.TaskDefinition == nil || out.TaskDefinition.TaskDefinitionArn == nil || out.TaskDefinition.Status == nil || out.TaskDefinition.Revision == nil {
		return "", 0, fmt.Errorf("unable to read task definition arn, status or revision")
	}

	if *out.TaskDefinition.Status == ecs.TaskDefinitionStatusActive {
		return *out.TaskDefinition.TaskDefinitionArn, *out.TaskDefinition.Revision, nil
	}
	return "", 0, nil
}

func findTask(clusterArn string, family string, sess *session.Session) (string, error) {
	logger := slog.With("family", family, "cluster", clusterArn)
	svc := ecs.New(sess)
	in := &ecs.ListTasksInput{
		Cluster: aws.String(clusterArn),
		Family:  aws.String(family),
	}
	out, err := svc.ListTasks(in)
	if err != nil {
		return "", fmt.Errorf("list tasks: %w", err)
	}

	if out == nil {
		return "", fmt.Errorf("list tasks gave no result")
	}

	if len(out.TaskArns) == 0 {
		logger.Debug("no tasks found")
		return "", nil // assume it does not exist, so no error
	}

	if len(out.TaskArns) > 1 {
		return "", fmt.Errorf("list tasks returned unexpected number of tasks: %d", len(out.TaskArns))
	}

	if out.TaskArns[0] == nil {
		return "", fmt.Errorf("list tasks returned nil task arn")
	}

	return *out.TaskArns[0], nil
}

func findQueue(queueName string, sess *session.Session) (string, string, error) {
	logger := slog.With("queue_name", queueName)
	sqssvc := sqs.New(sess)
	in := &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	}

	out, err := sqssvc.GetQueueUrl(in)
	if err != nil {
		if sqsIsQueueDoesNotExist(err) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("get queue url: %w", err)
	}

	if out == nil || out.QueueUrl == nil {
		logger.Debug("no queue url found")
		return "", "", nil // assume it does not exist, so no error
	}

	ina := &sqs.GetQueueAttributesInput{
		AttributeNames: []*string{
			aws.String("QueueArn"),
		},
		QueueUrl: out.QueueUrl,
	}
	outa, err := sqssvc.GetQueueAttributes(ina)
	if err != nil {
		return "", "", fmt.Errorf("get queue attributes: %w", err)
	}

	if outa == nil || len(outa.Attributes) == 0 {
		return "", "", fmt.Errorf("no queue attributes found")
	}

	queueArn, ok := outa.Attributes["QueueArn"]
	if !ok || queueArn == nil {
		return "", "", fmt.Errorf("no queue arn found")
	}

	return *queueArn, *out.QueueUrl, nil
}

func findSubscription(topicArn string, queueArn string, sess *session.Session) (string, error) {
	logger := slog.With("topic_arn", topicArn, "queue_arn", queueArn)
	logger.Debug("finding subscription")
	snssvc := sns.New(sess)
	in := &sns.ListSubscriptionsByTopicInput{
		TopicArn: aws.String(topicArn),
	}

	out, err := snssvc.ListSubscriptionsByTopic(in)
	if err != nil {
		return "", fmt.Errorf("list subsciption by topic: %w", err)
	}

	if out == nil || len(out.Subscriptions) == 0 {
		logger.Debug("no subscriptions found")
		return "", nil // assume it does not exist, so no error
	}

	for _, s := range out.Subscriptions {
		if s == nil || s.Endpoint == nil {
			continue
		}
		if *s.Endpoint == queueArn {
			if s.SubscriptionArn == nil {
				return "", fmt.Errorf("no subscription arn found")
			}
			return *s.SubscriptionArn, nil
		}
	}

	logger.Debug("no matching subscription found")
	if out.NextToken != nil {
		return "", fmt.Errorf("more subscriptions were available but pagination is not implemented yet")
	}

	return "", nil
}

func isTaskRunning(ctx context.Context, sess *session.Session, clusterArn, taskArn string) (bool, error) {
	logger := slog.With("task", taskArn, "cluster", clusterArn)
	svc := ecs.New(sess)
	in := &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterArn),
		Tasks: []*string{
			aws.String(taskArn),
		},
	}

	out, err := svc.DescribeTasks(in)
	if err != nil {
		return false, fmt.Errorf("describe tasks: %w", err)
	}
	for _, ta := range out.Tasks {
		if ta.TaskArn != nil && *ta.TaskArn == taskArn {
			if ta.LastStatus != nil && *ta.LastStatus == "RUNNING" {
				return true, nil
			}
			if ta.LastStatus == nil {
				logger.Debug("task status found but not ready", "status", "<nil>")
			} else {
				logger.Debug("task status found but not ready", "status", *ta.LastStatus)
			}
			return false, nil
		}
	}

	slog.Debug("task status not found")
	return false, nil
}

func isTaskStoppedOrStopping(ctx context.Context, sess *session.Session, clusterArn, taskArn string) (bool, error) {
	logger := slog.With("task", taskArn, "cluster", clusterArn)
	svc := ecs.New(sess)
	in := &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterArn),
		Tasks: []*string{
			aws.String(taskArn),
		},
	}

	out, err := svc.DescribeTasks(in)
	if err != nil {
		return false, fmt.Errorf("describe tasks: %w", err)
	}
	for _, ta := range out.Tasks {
		if ta.TaskArn != nil && *ta.TaskArn == taskArn {
			if ta.LastStatus != nil &&
				(*ta.LastStatus == "DEACTIVATING" ||
					*ta.LastStatus == "STOPPING" ||
					*ta.LastStatus == "DEPROVISIONING" ||
					*ta.LastStatus == "STOPPED" ||
					*ta.LastStatus == "DELETED") {
				return true, nil
			}
			if ta.LastStatus == nil {
				logger.Debug("task status found but not stopping", "status", "<nil>")
			} else {
				logger.Debug("task status found but not stopping", "status", *ta.LastStatus)
			}
			return false, nil
		}
	}

	slog.Debug("task status not found")
	return false, nil
}
