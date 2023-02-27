package api

import (
	"time"
)

const (
	ResourceTypeEcsTask            = "ecs_task"
	ResourceTypeEcsTaskDefinition  = "ecs_task_definition"
	ResourceTypeEcsSnsSubscription = "sns_subscription"
	ResourceTypeSqsQueue           = "sqs_queue"
	ResourceTypeEc2Instance        = "ec2_instance"
)

const (
	ResourceKeyArn           = "arn"
	ResourceKeyEcsClusterArn = "ecs_cluster_arn"
	ResourceKeyQueueURL      = "queue_url"
	ResourceKeyEc2InstanceID = "ecs_instance_id"
)

type NewExperimentInput struct {
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

type NewExperimentOutput struct {
	Message   string `json:"message"`
	URL       string `json:"url"`
	StatusURL string `json:"status_url"`
}

type ListExperimentsOutput struct {
	Items []ListExperimentsItem `json:"items"`
}

type ListExperimentsItem struct {
	Name    string    `json:"name"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Stopped time.Time `json:"stopped"`
}

type ExperimentStatusOutput struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Stopped time.Time `json:"stopped"`
	Status  string    `json:"status"`
}

type DeleteExperimentOutput struct{}

type GetExperimentOutput struct {
	Name       string    `json:"name"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Stopped    time.Time `json:"stopped"`
	Definition string    `json:"definition"`
}
