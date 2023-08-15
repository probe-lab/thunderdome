package infra

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
)

func mapsToKeyValuePair(ms ...map[string]string) []*ecs.KeyValuePair {
	size := 0
	for _, m := range ms {
		size += len(m)
	}

	pairs := make([]*ecs.KeyValuePair, 0, size)
	for _, m := range ms {
		for n, v := range m {
			pairs = append(pairs, &ecs.KeyValuePair{Name: aws.String(n), Value: aws.String(v)})
		}
	}
	return pairs
}

func ecsTags(m map[string]*string) []*ecs.Tag {
	tags := make([]*ecs.Tag, 0, len(m))
	for k, v := range m {
		tags = append(tags, &ecs.Tag{
			Key:   aws.String(k),
			Value: v,
		})
	}
	return tags
}

func sqsIsQueueDoesNotExist(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		return aerr.Code() == sqs.ErrCodeQueueDoesNotExist
	}
	return false
}

func sqsIsQueueDeletedRecently(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		return aerr.Code() == sqs.ErrCodeQueueDeletedRecently
	}
	return false
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

func deregisterEcsTaskDefinition(ctx context.Context, sess *session.Session, familyRevision string) error {
	in := &ecs.DeregisterTaskDefinitionInput{
		TaskDefinition: aws.String(familyRevision),
	}

	svc := ecs.New(sess)
	_, err := svc.DeregisterTaskDefinition(in)
	if err != nil {
		return fmt.Errorf("deregister task definition: %w", err)
	}
	return nil
}

func unsubscribeSqsQueue(ctx context.Context, sess *session.Session, requestSubscriptionArn string) error {
	snssvc := sns.New(sess)

	in := &sns.UnsubscribeInput{
		SubscriptionArn: aws.String(requestSubscriptionArn),
	}

	_, err := snssvc.Unsubscribe(in)
	if err != nil {
		return fmt.Errorf("unsubscribe from request topic: %w", err)
	}

	return nil
}

func deleteSqsQueue(ctx context.Context, sess *session.Session, requestQueueURL string) error {
	svc := sqs.New(sess)

	in := &sqs.DeleteQueueInput{
		QueueUrl: aws.String(requestQueueURL),
	}

	_, err := svc.DeleteQueue(in)
	if err != nil {
		return fmt.Errorf("delete queue: %w", err)
	}
	return nil
}

func dstr(v *string) string {
	if v == nil {
		return "<nil>"
	}
	return *v
}
