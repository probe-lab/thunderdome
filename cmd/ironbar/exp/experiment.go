package exp

import (
	"time"
)

type ExperimentJSON struct {
	Name     string `json:"name"`
	Duration int    `json:"duration"` // in hours

	MaxRequestRate int    `json:"max_request_rate"` // maximum number of requests per second to send to targets
	MaxConcurrency int    `json:"max_concurrency"`  // maximum number of concurrent requests to have in flight for each target
	RequestFilter  string `json:"request_filter"`   // filter to apply to incoming requests: "none", "pathonly", "validpathonly"

	DefaultImage      string   `json:"default_image"`      // image to use for targets by default, can be overriden
	SharedEnvironment []NVJSON `json:"shared_environment"` // environment variables provided to all targets

	DefaultInstanceType string `json:"default_instance_type"` // instance type to use for all targets, can be overriddden, default is "c6id.8xlarge"

	Targets []TargetJSON `json:"targets"`
}

type NVJSON struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TargetJSON struct {
	Name         string   `json:"name"`
	Image        string   `json:"image"`         // docker image to use. If empty, DefaultImage will be used instead
	InstanceType string   `json:"instance_type"` // instance type to use. If empty, DefaultInstanceType will be used instead
	Environment  []NVJSON `json:"environment"`   // additional environment variables
}

type Experiment struct {
	Name     string
	Duration time.Duration

	MaxRequestRate int
	MaxConcurrency int
	RequestFilter  string

	DefaultImage      string
	SharedEnvironment map[string]string

	DefaultInstanceType string

	Targets []*TargetDef
}

type TargetDef struct {
	Name         string
	Image        string
	InstanceType string
	Environment  map[string]string
}

func TestExperiment() *Experiment {
	return &Experiment{
		Name: "ironbar_test",

		Targets: []*TargetDef{
			{
				Name:  "target1",
				Image: "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0",
			},
		},
	}
}

/*
	{
		"name": "tweedles-2022-09-08",
		"duration": 24, // hours?


		"request_stream" : {
			"max_rate": 20,
			"max_concurrency": 100,
			"filter": "none",
		}

		"request_rate": 20,
		"request_concurrency"

		"request_filter"

		"default_target_image": // image to use by default, can be overriden
		"shared_environment": // environment variables used by all targets

		targets: [

			{
				"name": "target1",
				"image": "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0"
				"environment": [


				]
			}





		]
	}
*/
