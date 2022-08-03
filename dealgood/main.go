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
			Usage:       "Name of the experiment",
			Value:       "adhoc",
			Destination: &flags.experimentName,
			EnvVars:     []string{"DEALGOOD_EXPERIMENT"},
		},
		&cli.StringFlag{
			Name:        "experiment-file",
			Usage:       "Path to experiment JSON file",
			Destination: &flags.experimentFile,
			EnvVars:     []string{"DEALGOOD_EXPERIMENT_FILE"},
		},
		&cli.StringFlag{
			Name:        "source",
			Value:       "-",
			Usage:       "Name of request source, use '-' to read JSON from stdin, 'random' to use some builtin random requests",
			Destination: &flags.source,
			EnvVars:     []string{"DEALGOOD_SOURCE"},
		},
		&cli.StringFlag{
			Name:        "source-param",
			Usage:       "A parameter to be used with some sources",
			Destination: &flags.sourceParam,
			EnvVars:     []string{"DEALGOOD_SOURCE_PARAM"},
		},
		&cli.BoolFlag{
			Name:        "nogui",
			Usage:       "Disable GUI",
			Value:       false,
			Destination: &flags.nogui,
			EnvVars:     []string{"DEALGOOD_NOGUI"},
		},
		&cli.StringSliceFlag{
			Name:        "targets",
			Usage:       "Comma separated list of Base URLs of backends (if not using an experiment file)",
			Value:       cli.NewStringSlice("http://localhost:8080"),
			Destination: &flags.targets,
			EnvVars:     []string{"DEALGOOD_TARGETS"},
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
		&cli.StringFlag{
			Name:        "host",
			Usage:       "Force a host header to be sent with each request (if not using an experiment file)",
			Value:       "",
			Destination: &flags.host,
			EnvVars:     []string{"DEALGOOD_HOST"},
		},
		&cli.BoolFlag{
			Name:        "timings",
			Usage:       "Print timings for requests (not in gui mode)",
			Value:       true,
			Destination: &flags.timings,
			EnvVars:     []string{"DEALGOOD_TIMINGS"},
		},
		&cli.BoolFlag{
			Name:        "failures",
			Usage:       "Print failed request details to stderr (not in gui mode)",
			Value:       false,
			Destination: &flags.failures,
			EnvVars:     []string{"DEALGOOD_FAILURES"},
		},
		&cli.BoolFlag{
			Name:        "quiet",
			Usage:       "Suppress all output, overriding timings and failures flags (not in gui mode)",
			Value:       false,
			Destination: &flags.quiet,
			EnvVars:     []string{"DEALGOOD_QUIET"},
		},
	},
}

var flags struct {
	experimentName string
	experimentFile string
	source         string
	sourceParam    string
	nogui          bool
	targets        cli.StringSlice
	host           string
	rate           int
	concurrency    int
	duration       int
	timings        bool
	failures       bool
	quiet          bool
}

func main() {
	ctx := context.Background()
	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func Run(cc *cli.Context) error {
	if flags.quiet {
		flags.timings = false
		flags.failures = false
	}

	var err error
	var source RequestSource
	switch flags.source {
	case "random":
		source = NewRandomRequestSource(sampleRequests)
	case "nginxlog":
		source, err = NewNginxLogRequestSource(flags.sourceParam)
		if err != nil {
			return fmt.Errorf("nginx source: %w", err)
		}
	case "-":
		source = NewStdinRequestSource()
	default:
		return fmt.Errorf("unsupported source: %s", flags.source)
	}

	// Load the experiment definition or use a default one
	var exp ExperimentJSON
	if flags.experimentFile != "" {
		if err := readExperimentFile(flags.experimentFile, &exp); err != nil {
			return fmt.Errorf("read experiment file: %w", err)
		}
	} else {
		exp.Name = flags.experimentName
		exp.Rate = flags.rate
		exp.Concurrency = flags.concurrency
		exp.Duration = flags.duration
		for _, be := range flags.targets.Value() {
			exp.Backends = append(exp.Backends, &BackendJSON{
				BaseURL: be,
				Host:    flags.host,
			})
		}
	}

	if err := validateExperiment(&exp); err != nil {
		return fmt.Errorf("experiment: %w", err)
	}

	if flags.nogui {
		return nogui(cc.Context, source, &exp, !flags.quiet, flags.timings, flags.failures)
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
