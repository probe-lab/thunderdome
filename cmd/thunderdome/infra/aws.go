package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/iam"
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

func iamIsNoSuchEntity(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		return aerr.Code() == iam.ErrCodeNoSuchEntityException
	}
	return false
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
