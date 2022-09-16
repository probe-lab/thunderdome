package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/profile"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

const appName = "dealgood"

var app = &cli.App{
	Name:   appName,
	Action: Run,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "experiment",
			Usage:       "Name of the experiment",
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
			Usage:       "Name of request source, use '-' to read JSONL from stdin, 'random' to use some builtin random requests, 'loki' to read from a Loki log stream",
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
			Name:    "nogui",
			Usage:   "Disable GUI (deprecated and ignored)",
			Value:   true,
			EnvVars: []string{"DEALGOOD_NOGUI"},
		},
		&cli.StringSliceFlag{
			Name:        "targets",
			Usage:       "Comma separated list of Base URLs of targets, optionally each can be prefixed by a name, for example 'target::http://target.domain:8080' (if not using an experiment file)",
			Value:       cli.NewStringSlice("local::http://localhost:8080"),
			Destination: &flags.targets,
			EnvVars:     []string{"DEALGOOD_TARGETS"},
		},
		&cli.IntFlag{
			Name:        "rate",
			Usage:       "Number of requests per second to send (if not using an experiment file)",
			Value:       5,
			Destination: &flags.rate,
			EnvVars:     []string{"DEALGOOD_RATE"},
		},
		&cli.IntFlag{
			Name:        "concurrency",
			Usage:       "Number of concurrent requests to send (if not using an experiment file)",
			Value:       40,
			Destination: &flags.concurrency,
			EnvVars:     []string{"DEALGOOD_CONCURRENCY"},
		},
		&cli.IntFlag{
			Name:        "duration",
			Usage:       "Duration of experiment in seconds, -1 means forever (if not using an experiment file)",
			Value:       60,
			Destination: &flags.duration,
			EnvVars:     []string{"DEALGOOD_DURATION"},
		},
		&cli.StringFlag{
			Name:        "host",
			Usage:       "Force a host header to be sent with each request (if not using an experiment file)",
			Value:       "",
			Destination: &flags.hostHeader,
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
		&cli.StringFlag{
			Name:        "prometheus-addr",
			Usage:       "Network address to start a prometheus metric exporter server on (example: :9991)",
			Value:       "",
			Destination: &flags.prometheusAddr,
			EnvVars:     []string{"DEALGOOD_PROMETHEUS_ADDR"},
		},
		&cli.StringFlag{
			Name:        "cpuprofile",
			Usage:       "Write a CPU profile to the specified file before exiting.",
			Value:       "",
			Destination: &flags.cpuprofile,
			EnvVars:     []string{"DEALGOOD_CPUPROFILE"},
		},
		&cli.StringFlag{
			Name:        "memprofile",
			Usage:       "Write an allocation profile to the file before exiting.",
			Value:       "",
			Destination: &flags.memprofile,
			EnvVars:     []string{"DEALGOOD_MEMPROFILE"},
		},
		&cli.StringFlag{
			Name:        "loki-uri",
			Usage:       "URI of the loki server when using loki as a request source.",
			Value:       "",
			Destination: &flags.lokiURI,
			EnvVars:     []string{"DEALGOOD_LOKI_URI"},
		},
		&cli.StringFlag{
			Name:        "loki-username",
			Usage:       "Username to use when using loki as a request source.",
			Value:       "",
			Destination: &flags.lokiUsername,
			EnvVars:     []string{"DEALGOOD_LOKI_USERNAME"},
		},
		&cli.StringFlag{
			Name:        "loki-password",
			Usage:       "Password to use when using loki as a request source.",
			Value:       "",
			Destination: &flags.lokiPassword,
			EnvVars:     []string{"DEALGOOD_LOKI_PASSWORD"},
		},
		&cli.StringFlag{
			Name:        "loki-query",
			Usage:       "Query to use when using loki as a request source.",
			Value:       "",
			Destination: &flags.lokiQuery,
			EnvVars:     []string{"DEALGOOD_LOKI_QUERY"},
		},
		&cli.StringFlag{
			Name:        "sqs-queue",
			Usage:       "Name of the queue to subscribe to when using sqs as a request source.",
			Value:       "",
			Destination: &flags.sqsQueue,
			EnvVars:     []string{"DEALGOOD_SQS_QUEUE"},
		},
		&cli.BoolFlag{
			Name:        "interactive",
			Usage:       "Reduce all wait times and log summaries more frequently.",
			Value:       false,
			Destination: &flags.interactive,
			EnvVars:     []string{"DEALGOOD_INTERACTIVE"},
		},
		&cli.StringFlag{
			Name:        "filter",
			Usage:       "Filter to apply to requests from the request source (all, pathonly, validpathonly)",
			Value:       "pathonly",
			Destination: &flags.filter,
			EnvVars:     []string{"DEALGOOD_FILTER"},
		},
		&cli.StringFlag{
			Name:        "sqs-region",
			Usage:       "AWS region to use when connecting to sqs.",
			Value:       "eu-west-1",
			Destination: &flags.sqsRegion,
			EnvVars:     []string{"DEALGOOD_SQS_REGION"},
		},
		&cli.IntFlag{
			Name:        "summary-interval",
			Usage:       "Interval between printing measurement summaries.",
			Value:       300,
			Destination: &flags.summaryInterval,
			EnvVars:     []string{"DEALGOOD_SUMMARY_INTERVAL"},
		},
		&cli.StringFlag{
			Name:        "summary-type",
			Usage:       "Type of measurement summary to print (none, logbrief, dumpbrief, dumpfull).",
			Value:       "logbrief",
			Destination: &flags.summaryType,
			EnvVars:     []string{"DEALGOOD_SUMMARY_TYPE"},
		},
		&cli.IntFlag{
			Name:        "probe-timeout",
			Usage:       "Number of seconds to wait for targets to become ready.",
			Value:       300,
			Destination: &flags.probeTimeout,
			EnvVars:     []string{"DEALGOOD_PROBE_TIMEOUT"},
		},
		&cli.IntFlag{
			Name:        "pre-probe-wait",
			Usage:       "Number of seconds to wait for targets to start before probing readiness.",
			Value:       300,
			Destination: &flags.preProbeWait,
			EnvVars:     []string{"DEALGOOD_PRE_PROBE_WAIT"},
		},
	},
}

var flags struct {
	experimentName  string
	experimentFile  string
	source          string
	sourceParam     string
	targets         cli.StringSlice
	hostHeader      string
	rate            int
	concurrency     int
	duration        int
	timings         bool
	failures        bool
	quiet           bool
	prometheusAddr  string
	cpuprofile      string
	memprofile      string
	lokiURI         string
	lokiUsername    string
	lokiPassword    string
	lokiQuery       string
	sqsQueue        string
	sqsRegion       string
	interactive     bool
	filter          string
	preProbeWait    int // seconds
	probeTimeout    int // seconds
	summaryInterval int // seconds
	summaryType     string
}

func main() {
	ctx := context.Background()
	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func Run(cc *cli.Context) error {
	ctx := cc.Context

	if flags.quiet {
		flags.timings = false
		flags.failures = false
		log.SetOutput(io.Discard)
	}
	if flags.source == "-" {
		flags.source = "stdin"
	}
	if flags.interactive {
		log.SetFlags(0)
		flags.preProbeWait = 0
		flags.probeTimeout = 60
		flags.summaryInterval = 5
	} else {
		log.SetFlags(log.LstdFlags | log.LUTC | log.Lshortfile)
	}

	// Load the experiment definition or use a default one
	var expjson ExperimentJSON
	if flags.experimentFile != "" {
		if err := readExperimentFile(flags.experimentFile, &expjson); err != nil {
			return fmt.Errorf("read experiment file: %w", err)
		}
	} else {
		expjson.Name = flags.experimentName
		expjson.Rate = flags.rate
		expjson.Concurrency = flags.concurrency
		expjson.Duration = flags.duration
		for _, be := range flags.targets.Value() {
			bej := &TargetJSON{
				BaseURL: be,
				Host:    flags.hostHeader,
			}
			if name, base, found := strings.Cut(be, "::"); found {
				bej.Name = name
				bej.BaseURL = base
			} else {
				bej.BaseURL = be
			}
			expjson.Targets = append(expjson.Targets, bej)
		}
	}

	exp, err := newExperiment(&expjson)
	if err != nil {
		return fmt.Errorf("experiment: %w", err)
	}

	var filter RequestFilter
	switch flags.filter {
	case "all":
		filter = NullRequestFilter
	case "pathonly":
		filter = PathRequestFilter
	case "validpathonly":
		filter = ValidPathRequestFilter
	default:
		return fmt.Errorf("unsupported filter: %s", flags.filter)
	}

	var summaryPrinter SummaryPrinter
	switch flags.summaryType {
	case "none":
		summaryPrinter = NullSummaryPrinter
	case "logbrief":
		summaryPrinter = LogBriefSummary
	case "dumpbrief":
		summaryPrinter = DumpBriefSummary
	case "dumpfull":
		summaryPrinter = DumpFullSummary
	default:
		return fmt.Errorf("unsupported summary type: %s", flags.summaryType)
	}

	if flags.summaryInterval < 0 {
		return fmt.Errorf("summary interval must be zero or greater")
	}

	rg := &RunGroup{}

	metricLabels := map[string]string{
		"experiment": exp.Name,
		"source":     flags.source,
	}
	metrics, err := NewRequestSourceMetrics(metricLabels)
	if err != nil {
		return fmt.Errorf("new request source metrics: %w", err)
	}

	var source RequestSource
	switch flags.source {
	case "random":
		source = NewRandomRequestSource(filter, metrics, sampleRequests())
	case "nginxlog":
		source, err = NewNginxLogRequestSource(flags.sourceParam, filter, metrics)
		if err != nil {
			return fmt.Errorf("nginx source: %w", err)
		}
	case "loki":
		cfg := &LokiConfig{
			URI:      flags.lokiURI,
			Username: flags.lokiUsername,
			Password: flags.lokiPassword,
			Query:    flags.lokiQuery,
		}

		source, err = NewLokiRequestSource(cfg, filter, metrics, exp.Rate)
		if err != nil {
			return fmt.Errorf("loki source: %w", err)
		}
	case "sqs":
		awscfg := aws.NewConfig()
		awscfg.Region = aws.String(flags.sqsRegion)
		awscfg.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
			Timeout: 10 * time.Second,
		})

		cfg := &SQSConfig{
			AWSConfig: awscfg,
			Queue:     flags.sqsQueue,
		}

		source, err = NewSQSRequestSource(cfg, filter, metrics, exp.Rate)
		if err != nil {
			return fmt.Errorf("sqs source: %w", err)
		}
	case "stdin":
		source = NewStdinRequestSource(filter, metrics)
	default:
		return fmt.Errorf("unsupported source: %s", flags.source)
	}
	rg.Add(Restartable{source})

	if flags.prometheusAddr != "" {
		ps, err := NewPrometheusServer(flags.prometheusAddr)
		if err != nil {
			return fmt.Errorf("start prometheus: %w", err)
		}
		rg.Add(Restartable{ps})
	}

	if err := setupTracing(ctx); err != nil {
		return fmt.Errorf("set tracer provider: %w", err)
	}

	// timings
	timings := make(chan *RequestTiming, 10000)

	coll, err := NewCollector(timings, time.Duration(flags.summaryInterval)*time.Second, summaryPrinter)
	if err != nil {
		return fmt.Errorf("new collector: %w", err)
	}
	rg.Add(Restartable{coll})

	loader, err := NewLoader(exp.Name, exp.Targets, source, timings, exp.Rate, exp.Concurrency, exp.Duration)
	if err != nil {
		return fmt.Errorf("new loader: %w", err)
	}
	loader.PrintFailures = flags.failures

	if flags.probeTimeout > 0 {
		// Don't start ther loader until target readiness probe has completed
		rg.Add(Conditional{
			Pre: func(ctx context.Context) error {
				if err := targetsReady(ctx, exp.Targets, time.Duration(flags.preProbeWait)*time.Second, time.Duration(flags.probeTimeout)*time.Second); err != nil {
					return fmt.Errorf("targets ready check: %w", err)
				}
				return nil
			},
			Runnable: Restartable{loader},
		})
	} else {
		rg.Add(Restartable{loader})
	}

	if flags.cpuprofile != "" {
		defer profile.Start(profile.CPUProfile, profile.ProfileFilename(flags.cpuprofile)).Stop()
	}

	if flags.memprofile != "" {
		defer profile.Start(profile.MemProfile, profile.ProfileFilename(flags.memprofile)).Stop()
	}

	log.Printf("Starting experiment %s", exp.Name)
	log.Printf("Experiment duration: %s", secondsDesc(exp.Duration))
	log.Printf("Maximum request rate: %d", exp.Rate)
	log.Printf("Maximum request concurrency: %d", exp.Concurrency)
	log.Printf("Request filter: %s", flags.filter)
	log.Printf("Request source: %s", flags.source)
	switch flags.source {
	case "sqs":
		log.Printf("SQS queue: %s", flags.sqsQueue)
		log.Printf("SQS region: %s", flags.sqsRegion)
	case "loki":
		log.Printf("Loki URI: %s", flags.lokiURI)
		log.Printf("Loki query: %s", flags.lokiQuery)
		log.Printf("Loki username set: %v", len(flags.lokiUsername) > 0)
		log.Printf("Loki password set: %v", len(flags.lokiPassword) > 0)
	}

	if flags.hostHeader != "" {
		log.Printf("Forcing host header: %s", flags.hostHeader)
	}
	for _, t := range exp.Targets {
		fmt.Printf("Target %s (%s://%s)\n", t.Name, t.URLScheme, t.HostPort())
	}

	return rg.RunAndWait(ctx)
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

type Runnable interface {
	// Run starts running the component and blocks until the context is canceled, Shutdown is // called or a fatal error is encountered.
	Run(context.Context) error
}

type RunGroup struct {
	runnables []Runnable
}

func (a *RunGroup) Add(r Runnable) {
	a.runnables = append(a.runnables, r)
}

func (a *RunGroup) RunAndWait(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)

	for i := range a.runnables {
		r := a.runnables[i]
		g.Go(func() error { return r.Run(ctx) })
	}

	// Ensure components stop if we receive a terminating operating system signal.
	g.Go(func() error {
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, syscall.SIGTERM, syscall.SIGINT)
		select {
		case <-interrupt:
			log.Print("caught interrupt signal, cancelling all operations")
			cancel()
		case <-ctx.Done():
		}
		return nil
	})

	// Wait for all servers to run to completion.
	if err := g.Wait(); err != nil {
		if !errors.Is(err, context.Canceled) {
			return err
		}
	}
	return nil
}

type Restartable struct {
	Runnable
}

func (r Restartable) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := r.Runnable.Run(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
		}
	}
}

// Conditional runs a runnable only if the pre function returns nil
type Conditional struct {
	Pre func(ctx context.Context) error
	Runnable
}

func (c Conditional) Run(ctx context.Context) error {
	if err := c.Pre(ctx); err != nil {
		return err
	}
	return c.Runnable.Run(ctx)
}
