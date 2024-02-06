package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/profile"
	"github.com/urfave/cli/v2"

	"github.com/probe-lab/thunderdome/pkg/loki"
	"github.com/probe-lab/thunderdome/pkg/prom"
	"github.com/probe-lab/thunderdome/pkg/run"
)

const appName = "skyfish"

var app = &cli.App{
	Name:   appName,
	Action: Run,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "loki-uri",
			Usage:       "URI of the loki server when using loki as a request source.",
			Value:       "",
			Destination: &flags.lokiURI,
			EnvVars:     []string{"SKYFISH_LOKI_URI"},
		},
		&cli.StringFlag{
			Name:        "loki-username",
			Usage:       "Username to use when using loki as a request source.",
			Value:       "",
			Destination: &flags.lokiUsername,
			EnvVars:     []string{"SKYFISH_LOKI_USERNAME"},
		},
		&cli.StringFlag{
			Name:        "loki-password",
			Usage:       "Password to use when using loki as a request source.",
			Value:       "",
			Destination: &flags.lokiPassword,
			EnvVars:     []string{"SKYFISH_LOKI_PASSWORD"},
		},
		&cli.StringFlag{
			Name:        "loki-query",
			Usage:       "Query to use when using loki as a request source.",
			Value:       "",
			Destination: &flags.lokiQuery,
			EnvVars:     []string{"SKYFISH_LOKI_QUERY"},
		},
		&cli.StringFlag{
			Name:        "sns-topic",
			Usage:       "ARN of sns topic to publish to.",
			Value:       "",
			Destination: &flags.topicArn,
			EnvVars:     []string{"SKYFISH_TOPIC"},
		},
		&cli.StringFlag{
			Name:        "sns-region",
			Usage:       "AWS region to use when connecting to sns.",
			Value:       "eu-west-1",
			Destination: &flags.snsRegion,
			EnvVars:     []string{"SKYFISH_SNS_REGION"},
		},
		&cli.StringFlag{
			Name:        "prometheus-addr",
			Usage:       "Network address to start a prometheus metric exporter server on (example: :9991)",
			Value:       "",
			Destination: &flags.prometheusAddr,
			EnvVars:     []string{"SKYFISH_PROMETHEUS_ADDR"},
		},
		&cli.StringFlag{
			Name:        "cpuprofile",
			Usage:       "Write a CPU profile to the specified file before exiting.",
			Value:       "",
			Destination: &flags.cpuprofile,
			EnvVars:     []string{"SKYFISH_CPUPROFILE"},
		},
		&cli.StringFlag{
			Name:        "memprofile",
			Usage:       "Write an allocation profile to the file before exiting.",
			Value:       "",
			Destination: &flags.memprofile,
			EnvVars:     []string{"SKYFISH_MEMPROFILE"},
		},
	},
}

var flags struct {
	prometheusAddr string
	cpuprofile     string
	memprofile     string
	lokiURI        string
	lokiUsername   string
	lokiPassword   string
	lokiQuery      string
	topicArn       string
	snsRegion      string
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

	cfg := &loki.LokiConfig{
		AppName:  appName,
		URI:      flags.lokiURI,
		Username: flags.lokiUsername,
		Password: flags.lokiPassword,
		Query:    flags.lokiQuery,
	}

	rg := new(run.Group)

	source, err := loki.NewLokiTailer(cfg)
	if err != nil {
		return fmt.Errorf("loki source: %w", err)
	}
	rg.Add(source)

	if flags.prometheusAddr != "" {
		ps, err := prom.NewPrometheusServer(flags.prometheusAddr, "/metrics", appName)
		if err != nil {
			return fmt.Errorf("start prometheus: %w", err)
		}
		rg.Add(Restartable{ps})
	}

	awscfg := aws.NewConfig()
	awscfg.Region = aws.String(flags.snsRegion)
	awscfg.WithHTTPClient(&http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
		Timeout: 10 * time.Second,
	})
	publisher, err := NewPublisher(awscfg, flags.topicArn, source.Chan())
	if err != nil {
		return fmt.Errorf("new publisher: %w", err)
	}
	rg.Add(publisher)

	rg.Add(new(Health))

	if flags.cpuprofile != "" {
		defer profile.Start(profile.CPUProfile, profile.ProfileFilename(flags.cpuprofile)).Stop()
	}

	if flags.memprofile != "" {
		defer profile.Start(profile.MemProfile, profile.ProfileFilename(flags.memprofile)).Stop()
	}

	return rg.RunAndWait(ctx)
}

type Restartable struct {
	run.Runnable
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
