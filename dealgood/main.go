package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/profile"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats/view"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
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
			Usage:       "Reduce all wait times and log timings more frequently.",
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
	},
}

var flags struct {
	experimentName string
	experimentFile string
	source         string
	sourceParam    string
	targets        cli.StringSlice
	hostHeader     string
	rate           int
	concurrency    int
	duration       int
	timings        bool
	failures       bool
	quiet          bool
	prometheusAddr string
	cpuprofile     string
	memprofile     string
	lokiURI        string
	lokiUsername   string
	lokiPassword   string
	lokiQuery      string
	sqsQueue       string
	sqsRegion      string
	interactive    bool
	filter         string
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC | log.Lshortfile)
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
	}
	if flags.source == "-" {
		flags.source = "stdin"
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

	if flags.prometheusAddr != "" {
		if err := startPrometheusServer(flags.prometheusAddr); err != nil {
			return fmt.Errorf("start prometheus: %w", err)
		}
	}

	if flags.cpuprofile != "" {
		defer profile.Start(profile.CPUProfile, profile.ProfileFilename(flags.cpuprofile)).Stop()
	}

	if flags.memprofile != "" {
		defer profile.Start(profile.MemProfile, profile.ProfileFilename(flags.memprofile)).Stop()
	}

	tc := propagation.TraceContext{}
	otel.SetTextMapPropagator(tc)
	if err := setTracerProvider(ctx); err != nil {
		return fmt.Errorf("set tracer provider: %w", err)
	}

	if err := targetsReady(ctx, exp.Targets, flags.quiet, flags.interactive); err != nil {
		return fmt.Errorf("targets ready check: %w", err)
	}

	return nogui(ctx, source, exp, !flags.quiet, flags.timings, flags.failures, flags.interactive)
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

func startPrometheusServer(addr string) error {
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace:  appName,
		Registerer: prom.DefaultRegisterer,
		Gatherer:   prom.DefaultGatherer,
	})
	if err != nil {
		return fmt.Errorf("new prometheus exporter: %w", err)
	}

	// register prometheus with opencensus
	view.RegisterExporter(pe)
	view.SetReportingPeriod(2 * time.Second)

	mux := http.NewServeMux()
	mux.Handle("/metrics", pe)
	go func() {
		http.ListenAndServe(addr, mux)
	}()
	return nil
}

func setTracerProvider(ctx context.Context) error {
	exporters, err := buildTracerExporters(ctx)
	if err != nil {
		return err
	}

	options := []trace.TracerProviderOption{}

	for _, exporter := range exporters {
		options = append(options, trace.WithBatcher(exporter))
	}

	tp := trace.NewTracerProvider(options...)
	otel.SetTracerProvider(tp)

	return nil
}

func buildTracerExporters(ctx context.Context) ([]trace.SpanExporter, error) {
	var exporters []trace.SpanExporter

	if os.Getenv("OTEL_TRACES_EXPORTER") == "" {
		return exporters, nil
	}

	for _, exporterStr := range strings.Split(os.Getenv("OTEL_TRACES_EXPORTER"), ",") {
		switch exporterStr {
		case "otlp":
			exporter, err := otlptracegrpc.New(ctx)
			if err != nil {
				return nil, fmt.Errorf("new OTLP gRPC exporter: %w", err)
			}
			exporters = append(exporters, exporter)
		case "jaeger":
			exporter, err := jaeger.New(jaeger.WithCollectorEndpoint())
			if err != nil {
				return nil, fmt.Errorf("new Jaeger exporter: %w", err)
			}
			exporters = append(exporters, exporter)
		case "zipkin":
			exporter, err := zipkin.New("")
			if err != nil {
				return nil, fmt.Errorf("new Zipkin exporter: %w", err)
			}
			exporters = append(exporters, exporter)
		default:
			return nil, fmt.Errorf("unknown or unsupported exporter: %q", exporterStr)
		}
	}
	return exporters, nil
}
