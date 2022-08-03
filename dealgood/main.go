package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v2"
)

const appName = "dealgood"

var app = &cli.App{
	Name:   appName,
	Action: Run,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "experiment",
			Aliases:     []string{"x"},
			Usage:       "Path to experiment JSON file",
			Destination: &flags.experiment,
			EnvVars:     []string{"DEALGOOD_EXPERIMENT"},
		},
		&cli.BoolFlag{
			Name:        "nogui",
			Usage:       "Disable GUI",
			Value:       true,
			Destination: &flags.nogui,
			EnvVars:     []string{"DEALGOOD_NOGUI"},
		},
		&cli.StringFlag{
			Name:        "baseurl",
			Usage:       "Base URL of backend (if not using an experiment file)",
			Value:       "http://localhost:8080",
			Destination: &flags.baseURL,
			EnvVars:     []string{"DEALGOOD_BASEURL"},
		},
		&cli.IntFlag{
			Name:        "rate",
			Usage:       "Number of requests per second to send (if not using an experiment file)",
			Value:       100,
			Destination: &flags.rate,
			EnvVars:     []string{"DEALGOOD_RATE"},
		},
		&cli.IntFlag{
			Name:        "concurrency",
			Usage:       "Number of concurrent requests to send (if not using an experiment file)",
			Value:       8,
			Destination: &flags.concurrency,
			EnvVars:     []string{"DEALGOOD_CONCURRENCY"},
		},
		&cli.IntFlag{
			Name:        "duration",
			Usage:       "Duration of experiment in seconds(if not using an experiment file)",
			Value:       60,
			Destination: &flags.duration,
			EnvVars:     []string{"DEALGOOD_DURATION"},
		},
	},
}

var flags struct {
	experiment  string
	nogui       bool
	baseURL     string
	rate        int
	concurrency int
	duration    int
}

func main() {
	ctx := context.Background()
	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func Run(cc *cli.Context) error {
	source := NewRandomRequestSource(sampleRequests)

	// Load the experiment definition or use a default one
	var exp ExperimentJSON
	if flags.experiment != "" {
		if err := readExperimentFile(flags.experiment, &exp); err != nil {
			return fmt.Errorf("read experiment file: %w", err)
		}
	} else {
		exp.Name = "adhoc"
		exp.Rate = flags.rate
		exp.Concurrency = flags.concurrency
		exp.Duration = flags.duration
		exp.Backends = []*BackendJSON{
			{
				BaseURL: flags.baseURL,
			},
		}

	}

	if err := validateExperiment(&exp); err != nil {
		return fmt.Errorf("experiment: %w", err)
	}

	if flags.nogui {
		return nogui(cc.Context, source, &exp)
	}

	g, err := NewGui(source, &exp)
	if err != nil {
		return fmt.Errorf("gui: %w", err)
	}
	defer g.Close()
	return g.Show(cc.Context, 100*time.Millisecond)
}

func readExperimentFile(fname string, exp *ExperimentJSON) error {
	expf, err := os.Open(fname)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer expf.Close()

	if err := json.NewDecoder(expf).Decode(exp); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	return nil
}
