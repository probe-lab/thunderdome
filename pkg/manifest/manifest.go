package manifest

import (
	"time"
)

const (
	ResourceTypeEcsTask            = "ecs_task"
	ResourceTypeEcsTaskDefinition  = "ecs_task_definition"
	ResourceTypeEcsSnsSubscription = "sns_subscription"
	ResourceTypeSqsQueue           = "sqs_queue"
)

const (
	ResourceKeyArn            = "arn"
	ResourceKeyQueueURL       = "queue_url"
	ResourceKeyFamilyRevision = "family_revision"
)

type Manifest struct {
	Name       string     `json:"name"`
	Start      time.Time  `json:"start"`
	End        time.Time  `json:"end"`
	Definition string     `json:"definition"`
	Resources  []Resource `json:"resources"`
}

type Resource struct {
	Type string            `json:"type"`
	Keys map[string]string `json:"keys"`
}
