package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/plprobelab/thunderdome/pkg/exp"
)

type ExperimentJSON struct {
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	MaxRequestRate int           `json:"max_request_rate"` // maximum number of requests per second to send to targets
	MaxConcurrency int           `json:"max_concurrency"`  // maximum number of concurrent requests to have in flight for each target
	RequestFilter  string        `json:"request_filter"`   // filter to apply to incoming requests: "none", "pathonly", "validpathonly"
	Targets        []TargetJSON  `json:"targets"`
	Shared         *SharedJSON   `json:"shared"` // environment variables and init commands provided to all targets
	Defaults       *DefaultsJSON `json:"defaults"`
}

type NVJSON struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TargetJSON struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	InstanceType string   `json:"instance_type,omitempty"` // instance type to use. If empty, DefaultInstanceType will be used instead
	Environment  []NVJSON `json:"environment,omitempty"`   // additional environment variables

	BaseImage    string       `json:"base_image,omitempty"`
	BuildFromGit *GitSpecJSON `json:"build_from_git,omitempty"`
	// Commands that should be added to the container's container.init.d directory
	// for example: ipfs config --json Swarm.ConnMgr.GracePeriod '"2m"'
	InitCommands     []string `json:"init_commands,omitempty"`
	InitCommandsFrom string   `json:"init_commands_from,omitempty"`

	UseImage string `json:"use_image,omitempty"` // docker image to use. If empty, DefaultImage will be used instead. Must be pre-configured for thunderdome.
}

type DefaultsJSON struct {
	InstanceType     string       `json:"instance_type,omitempty"` // instance type to use. If empty, DefaultInstanceType will be used instead
	Environment      []NVJSON     `json:"environment,omitempty"`   // additional environment variables
	BaseImage        string       `json:"base_image,omitempty"`
	BuildFromGit     *GitSpecJSON `json:"build_from_git,omitempty"`
	InitCommands     []string     `json:"init_commands,omitempty"`
	InitCommandsFrom string       `json:"init_commands_from,omitempty"`
	UseImage         string       `json:"use_image,omitempty"` // docker image to use. If empty, DefaultImage will be used instead. Must be pre-configured for thunderdome.
}

type SharedJSON struct {
	Environment      []NVJSON `json:"environment,omitempty"`
	InitCommands     []string `json:"init_commands,omitempty"`
	InitCommandsFrom string   `json:"init_commands_from,omitempty"`
}

type GitSpecJSON struct {
	Repo   string `json:"repo,omitempty"`
	Commit string `json:"commit,omitempty"`
	Tag    string `json:"tag,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// Target name must contain only lowercase letters, numbers and hyphens and must start with a letter
var reTargetName = regexp.MustCompile(`^[a-z][a-z0-9-]+$`)

// Experiment name must contain only lowercase letters, numbers and hyphens and must start with a letter
var reExperimentName = regexp.MustCompile(`^[a-z][a-z0-9-]+$`)

func LoadExperiment(ctx context.Context, filename string) (*exp.Experiment, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dir := filepath.Dir(filename)

	e, err := ParseExperiment(ctx, f, dir)
	if err != nil {
		return nil, fmt.Errorf("parse experiment definition: %w", err)
	}

	return e, nil
}

func ParseExperiment(ctx context.Context, r io.Reader, baseDir string) (*exp.Experiment, error) {
	ej := new(ExperimentJSON)

	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	if err := dec.Decode(ej); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}

	if !reExperimentName.MatchString(ej.Name) {
		return nil, fmt.Errorf("experiment name must start with a letter and contain only lowercase letters, numbers and hyphens: %q", ej.Name)
	}

	e := &exp.Experiment{
		Name: ej.Name,
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

	if ej.Shared.InitCommandsFrom != "" {
		if len(ej.Shared.InitCommands) > 0 {
			return nil, fmt.Errorf("cannot specify both init_commands and init_commands_from for target shared config")
		}

		fromFile := filepath.Join(baseDir, ej.Shared.InitCommandsFrom)
		content, err := os.ReadFile(fromFile)
		if err != nil {
			return nil, fmt.Errorf("failed reading init_commands_from in target shared config: %w", err)
		}
		ej.Shared.InitCommands = []string{string(content)}
	}

	if ej.Defaults.InitCommandsFrom != "" {
		if len(ej.Defaults.InitCommands) > 0 {
			return nil, fmt.Errorf("cannot specify both init_commands and init_commands_from for target default config")
		}
		fromFile := filepath.Join(baseDir, ej.Defaults.InitCommandsFrom)
		content, err := os.ReadFile(fromFile)
		if err != nil {
			return nil, fmt.Errorf("failed reading init_commands_from in target default config: %w", err)
		}
		ej.Defaults.InitCommands = []string{string(content)}
	}

	uniqueNames := map[string]bool{}
	for i, tj := range ej.Targets {
		t := &exp.TargetSpec{
			Environment: map[string]string{},
		}
		if tj.Name != "" {
			t.Name = tj.Name
		} else {
			return nil, fmt.Errorf("name must be supplied for target %d", i+1)
		}

		if tj.InitCommandsFrom != "" {
			if len(tj.InitCommands) > 0 {
				return nil, fmt.Errorf("cannot specify both init_commands and init_commands_from for target %d", i+1)
			}
			fromFile := filepath.Join(baseDir, tj.InitCommandsFrom)
			content, err := os.ReadFile(fromFile)
			if err != nil {
				return nil, fmt.Errorf("failed reading init_commands_from in target %d: %w", i+1, err)
			}
			tj.InitCommands = []string{string(content)}
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
				t.ImageSpec = &exp.ImageSpec{
					BaseImage: tj.BaseImage,
				}
			} else if tj.BuildFromGit != nil {
				if nonEmptyCount(tj.BuildFromGit.Commit, tj.BuildFromGit.Tag, tj.BuildFromGit.Branch) != 1 {
					return nil, fmt.Errorf("must only specifiy one of commit, tag or branch in for target %s", tj.Name)
				}
				t.ImageSpec = &exp.ImageSpec{
					Git: &exp.GitSpec{
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
				t.ImageSpec = &exp.ImageSpec{
					BaseImage: ej.Defaults.BaseImage,
				}
			} else if ej.Defaults != nil && ej.Defaults.BuildFromGit != nil {
				if nonEmptyCount(ej.Defaults.BuildFromGit.Commit, ej.Defaults.BuildFromGit.Tag, ej.Defaults.BuildFromGit.Branch) != 1 {
					return nil, fmt.Errorf("must only specifiy one of commit, tag or branch in for target defaults")
				}
				t.ImageSpec = &exp.ImageSpec{
					Git: &exp.GitSpec{
						Repo:   ej.Defaults.BuildFromGit.Repo,
						Commit: ej.Defaults.BuildFromGit.Commit,
						Tag:    ej.Defaults.BuildFromGit.Tag,
						Branch: ej.Defaults.BuildFromGit.Branch,
					},
				}
			} else {
				return nil, fmt.Errorf("must specify one of use_image, base_image or build_from_git in target %s definition", tj.Name)
			}

			t.ImageSpec.Description = tj.Description

			// combine init commands
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
