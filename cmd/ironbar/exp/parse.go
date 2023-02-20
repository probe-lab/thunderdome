package exp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// Target name must contain only lowercase letters, numbers and hyphens and must start with a letter
var reTargetName = regexp.MustCompile(`^[a-z][a-z0-9-]+$`)

func Parse(ctx context.Context, r io.Reader) (*Experiment, error) {
	ej := new(ExperimentJSON)
	if err := json.NewDecoder(r).Decode(ej); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}

	e := &Experiment{
		Name: ej.Name,
	}

	if ej.Duration > 0 {
		e.Duration = time.Duration(ej.Duration) * time.Minute
	} else {
		return nil, fmt.Errorf("duration must be a positive number of minutes")
	}

	if ej.MaxRequestRate > 0 {
		e.MaxRequestRate = ej.MaxRequestRate
	} else {
		return nil, fmt.Errorf("max request rate must be a positive number")
	}

	if ej.MaxConcurrency > 0 {
		e.MaxConcurrency = ej.MaxConcurrency
	} else {
		return nil, fmt.Errorf("max concurrency must be a positive number")
	}

	switch ej.RequestFilter {
	case "none", "pathonly", "validpathonly":
		e.RequestFilter = ej.RequestFilter
	default:
		return nil, fmt.Errorf("unsupported request filter")
	}

	uniqueNames := map[string]bool{}
	for i, tj := range ej.Targets {
		t := &TargetSpec{
			Environment: map[string]string{},
		}
		if tj.Name != "" {
			t.Name = tj.Name
		} else {
			return nil, fmt.Errorf("name must be supplied for target %d", i+1)
		}

		if uniqueNames[t.Name] {
			return nil, fmt.Errorf("target name must be unique, %q has already been used", t.Name)
		}
		uniqueNames[t.Name] = true

		if !reTargetName.MatchString(t.Name) {
			return nil, fmt.Errorf("target name must start with a letter and contain only lowercase letters, numbers and hyphens: %q", t.Name)
		}

		if tj.InstanceType != "" {
			t.InstanceType = tj.InstanceType
		} else if ej.Defaults != nil && ej.Defaults.InstanceType != "" {
			t.InstanceType = ej.Defaults.InstanceType
		} else {
			return nil, fmt.Errorf("instance type must be supplied for target %d or a default specified", i+1)
		}

		// combine environment variables
		if ej.Shared != nil {
			for _, nv := range ej.Shared.Environment {
				t.Environment[nv.Name] = nv.Value
			}
		}
		if len(tj.Environment) > 0 {
			for _, nv := range tj.Environment {
				t.Environment[nv.Name] = nv.Value
			}
		} else if ej.Defaults != nil {
			for _, nv := range ej.Defaults.Environment {
				t.Environment[nv.Name] = nv.Value
			}
		}

		if tj.UseImage != "" {
			if tj.BaseImage != "" {
				return nil, fmt.Errorf("must not specify both use_image and base_image for target %s", tj.Name)
			}
			if tj.BuildFromGit != nil {
				return nil, fmt.Errorf("must not specify both use_image and build_from_git for target %s", tj.Name)
			}
			t.Image = tj.UseImage
		} else {
			if tj.BaseImage != "" {
				if tj.BuildFromGit != nil {
					return nil, fmt.Errorf("must not specify both base_image and build_from_git for target %s", tj.Name)
				}
				t.ImageSpec = &ImageSpec{
					BaseImage: tj.BaseImage,
				}
			} else if tj.BuildFromGit != nil {
				if nonEmptyCount(tj.BuildFromGit.Commit, tj.BuildFromGit.Tag, tj.BuildFromGit.Branch) != 1 {
					return nil, fmt.Errorf("must only specifiy one of commit, tag or branch in for target %s", tj.Name)
				}
				t.ImageSpec = &ImageSpec{
					Git: &GitSpec{
						Repo:   tj.BuildFromGit.Repo,
						Commit: tj.BuildFromGit.Commit,
						Tag:    tj.BuildFromGit.Tag,
						Branch: tj.BuildFromGit.Branch,
					},
				}
			} else if ej.Defaults != nil && ej.Defaults.UseImage != "" {
				if ej.Defaults.BaseImage != "" {
					return nil, fmt.Errorf("must not specify both use_image and base_image in target defaults")
				}
				if ej.Defaults.BuildFromGit != nil {
					return nil, fmt.Errorf("must not specify both use_image and build_from_git in target defaults")
				}
				t.Image = ej.Defaults.UseImage
			} else if ej.Defaults != nil && ej.Defaults.BaseImage != "" {
				if ej.Defaults.BuildFromGit != nil {
					return nil, fmt.Errorf("must not specify both base_image and build_from_git  in target defaults")
				}
				t.ImageSpec = &ImageSpec{
					BaseImage: ej.Defaults.BaseImage,
				}
			} else if ej.Defaults != nil && ej.Defaults.BuildFromGit != nil {
				if nonEmptyCount(ej.Defaults.BuildFromGit.Commit, ej.Defaults.BuildFromGit.Tag, ej.Defaults.BuildFromGit.Branch) != 1 {
					return nil, fmt.Errorf("must only specifiy one of commit, tag or branch in for target defaults")
				}
				t.ImageSpec = &ImageSpec{
					Git: &GitSpec{
						Repo:   ej.Defaults.BuildFromGit.Repo,
						Commit: ej.Defaults.BuildFromGit.Commit,
						Tag:    ej.Defaults.BuildFromGit.Tag,
						Branch: ej.Defaults.BuildFromGit.Branch,
					},
				}
			} else {
				return nil, fmt.Errorf("must specify one of use_image, base_image or build_from_git in target %s definition", tj.Name)
			}

			// combine environment variables
			if ej.Shared != nil {
				t.ImageSpec.InitCommands = append(t.ImageSpec.InitCommands, ej.Shared.InitCommands...)
			}
			if len(tj.InitCommands) > 0 {
				t.ImageSpec.InitCommands = append(t.ImageSpec.InitCommands, tj.InitCommands...)
			} else if ej.Defaults != nil {
				t.ImageSpec.InitCommands = append(t.ImageSpec.InitCommands, ej.Defaults.InitCommands...)
			}

		}
		e.Targets = append(e.Targets, t)
	}

	return e, nil
}

// nonEmptyCount returns the number the passed strings that are not empty
func nonEmptyCount(strs ...string) int {
	nonEmpty := 0
	for _, s := range strs {
		if len(s) > 0 {
			nonEmpty++
		}
	}
	return nonEmpty
}

func TestExperiment() *Experiment {
	e, err := Parse(context.Background(), strings.NewReader(testExperiment))
	if err != nil {
		panic(fmt.Sprintf("test experiment json invalid: %v", err))
	}
	return e
}

var testExperiment = `{
	"name": "ironbar_test",
	"duration": 30,
	"max_request_rate": 10,
	"max_concurrency": 100,
	"request_filter": "pathonly",

	"shared": {
		"environment": [
			{ "name": "IPFS_PROFILE", "value": "server" }
		]
	},

	"defaults": {
		"instance_type": "io_medium"
	},

	"targets": [
		{
			"name": "kubo-v0.15.0",
			"use_image": "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0"
		}
	]
}`

func TweedlesExperiment() *Experiment {
	e, err := Parse(context.Background(), strings.NewReader(tweedles))
	if err != nil {
		panic(fmt.Sprintf("tweedles experiment json invalid: %v", err))
	}
	return e
}

var tweedles = `{
	"name": "tweedles-ironbar",
	"duration": 30,
	"max_request_rate": 20,
	"max_concurrency": 100,
	"request_filter": "pathonly",

	"defaults": {
		"instance_type": "io_medium",
		"base_image": "ipfs/kubo:v0.18.1"
	},

	"shared": {
		"init_commands" : [
			"ipfs config --json AutoNAT '{\"ServiceMode\": \"disabled\"}'",
			"ipfs config --json Datastore.BloomFilterSize '268435456'",
			"ipfs config --json Datastore.StorageGCWatermark 90",
			"ipfs config --json Datastore.StorageMax '\"160GB\"'",
			"ipfs config --json Pubsub.StrictSignatureVerification false",
			"ipfs config --json Reprovider.Interval '\"0\"'",
			"ipfs config --json Routing.Type '\"dhtserver\"'",
			"ipfs config --json Swarm.ConnMgr.GracePeriod '\"2m\"'",
			"ipfs config --json Swarm.ConnMgr.HighWater 5000",
			"ipfs config --json Swarm.ConnMgr.LowWater 3000",
			"ipfs config --json Swarm.ConnMgr.DisableBandwidthMetrics true",
			"ipfs config --json Experimental.AcceleratedDHTClient true",
			"ipfs config --json Experimental.StrategicProviding true"
		]
	},

	"targets": [
		{
			"name": "dee"
		},
		{
			"name": "dum"
		}
	]
}
`
