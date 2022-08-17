package main

import (
	"fmt"
	"net/url"
)

type ExperimentJSON struct {
	Name        string        `json:"name"`
	Rate        int           `json:"rate"`        // maximum number of requests per second per target
	Concurrency int           `json:"concurrency"` // number of concurrent requests per target
	Duration    int           `json:"duration"`    // suggested duration of the experiment in seconds
	Targets     []*TargetJSON `json:"targets"`
}

type TargetJSON struct {
	Name    string `json:"name"`           // short name of the target to be used in reports
	BaseURL string `json:"base_url"`       // base URL of the target (without a path)
	Host    string `json:"host,omitempty"` // An optional hostname to be sent as a Host header in requests
}

func validateExperiment(exp *ExperimentJSON) error {
	if exp.Rate <= 0 {
		return fmt.Errorf("rate must be greater than zero")
	}
	if exp.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than zero")
	}
	if exp.Duration <= 0 && exp.Duration != -1 {
		return fmt.Errorf("duration must be -1 or greater than zero ")
	}

	if len(exp.Targets) == 0 {
		return fmt.Errorf("at least one target must be specified")
	}

	seenNames := map[string]bool{}
	for i, be := range exp.Targets {
		if be.BaseURL == "" {
			return fmt.Errorf("target %d must have a base url", i+1)
		}

		u, err := url.Parse(be.BaseURL)
		if err != nil {
			return fmt.Errorf("target %d must have a valid base url: %w", i+1, err)
		}

		if u.Path != "" {
			return fmt.Errorf("target %d base url should not have a path", i+1)
		}

		if be.Name == "" {
			be.Name = u.Hostname()
		}

		if seenNames[be.Name] {
			return fmt.Errorf("duplicate target name found: %s", be.Name)
		}
		seenNames[be.Name] = true

	}

	if exp.Name == "" {
		return fmt.Errorf("experiment name must be specified")
	}

	return nil
}
