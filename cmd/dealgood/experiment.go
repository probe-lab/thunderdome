package main

import (
	"fmt"
	"net/url"
	"sync"

	"github.com/probe-lab/thunderdome/pkg/request"
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

type Experiment struct {
	Name        string
	Rate        int
	Concurrency int
	Duration    int
	Targets     []*Target
}

type Target struct {
	Name        string                // short name of the target to be used in reports and metrics
	BaseURL     string                // base URL of the target (without a path)
	HostName    string                // the name of the host to be sent in the Host header of requests (may be different to the target's own host name)
	URLScheme   string                // http or https
	RawHostPort string                // hostname and port of target as derived from the URL
	Requests    chan *request.Request // channel used to receive requests to be issued to the target

	mu               sync.Mutex // guards accesses to hostPort which may change over time
	resolvedHostPort string
}

func (t *Target) HostPort() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.resolvedHostPort
}

func (t *Target) SetHostPort(hostport string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resolvedHostPort = hostport
}

func newExperiment(expjson *ExperimentJSON) (*Experiment, error) {
	if expjson.Name == "" {
		return nil, fmt.Errorf("experiment name must be specified")
	}
	if expjson.Rate <= 0 {
		return nil, fmt.Errorf("rate must be greater than zero")
	}
	if expjson.Concurrency <= 0 {
		return nil, fmt.Errorf("concurrency must be greater than zero")
	}
	if expjson.Duration <= 0 && expjson.Duration != -1 {
		return nil, fmt.Errorf("duration must be -1 or greater than zero ")
	}

	if len(expjson.Targets) == 0 {
		return nil, fmt.Errorf("at least one target must be specified")
	}

	exp := &Experiment{
		Name:        expjson.Name,
		Rate:        expjson.Rate,
		Concurrency: expjson.Concurrency,
		Duration:    expjson.Duration,
	}

	seenNames := map[string]bool{}
	for i, tj := range expjson.Targets {
		if tj.BaseURL == "" {
			return nil, fmt.Errorf("target %d must have a base url", i+1)
		}

		u, err := url.Parse(tj.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("target %d must have a valid base url: %w", i+1, err)
		}

		if u.Path != "" {
			return nil, fmt.Errorf("target %d base url should not have a path", i+1)
		}

		if tj.Name == "" {
			tj.Name = u.Hostname()
		}

		if seenNames[tj.Name] {
			return nil, fmt.Errorf("duplicate target name found: %s", tj.Name)
		}
		seenNames[tj.Name] = true

		t := &Target{
			Name:             tj.Name,
			BaseURL:          tj.BaseURL,
			HostName:         u.Hostname(),
			URLScheme:        u.Scheme,
			RawHostPort:      u.Host,
			resolvedHostPort: u.Host,
			Requests:         make(chan *request.Request),
		}

		// allow host to be overridden
		if tj.Host != "" {
			t.HostName = tj.Host
		}

		exp.Targets = append(exp.Targets, t)

	}

	return exp, nil
}
