package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/profile"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

const appName = "ironbar"

var app = &cli.App{
	Name:        appName,
	Usage:       "a tool for managing experiments",
	Description: "ironbar is a tool for managing experiments",
	Commands: []*cli.Command{
		StatusCommand,
		DeployCommand,
		ImageCommand,
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "prometheus-addr",
			Usage:       "Network address to start a prometheus metric exporter server on (example: :9991)",
			Value:       "",
			Destination: &commonFlags.prometheusAddr,
			EnvVars:     []string{"IRONBAR_PROMETHEUS_ADDR"},
		},
		&cli.StringFlag{
			Name:        "cpuprofile",
			Usage:       "Write a CPU profile to the specified file before exiting.",
			Value:       "",
			Destination: &commonFlags.cpuprofile,
			EnvVars:     []string{"IRONBAR_CPUPROFILE"},
		},
		&cli.StringFlag{
			Name:        "memprofile",
			Usage:       "Write an allocation profile to the file before exiting.",
			Value:       "",
			Destination: &commonFlags.memprofile,
			EnvVars:     []string{"IRONBAR_MEMPROFILE"},
		},
		&cli.StringFlag{
			Name:        "aws-region",
			Usage:       "AWS region to use when deploying experiments.",
			Value:       "",
			Destination: &commonFlags.awsRegion,
			EnvVars:     []string{"IRONBAR_AWS_REGION"},
		},
	},
}

var commonFlags struct {
	prometheusAddr string
	cpuprofile     string
	memprofile     string
	awsRegion      string
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	ctx := context.Background()
	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func RunTemp(cc *cli.Context) error {
	ctx := cc.Context

	experiment := "ironbar_test"

	log.Printf("starting deployment of experiment %q", experiment)

	base := NewBaseInfra(experiment, "eu-west-1", "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome")
	log.Printf("starting setup of base infra")
	if err := base.Setup(ctx); err != nil {
		return fmt.Errorf("failed to setup base infra: %w", err)
	}

	log.Printf("waiting for base infra to be ready")
	if err := WaitUntil(ctx, base.Ready, 2*time.Second, 30*time.Second); err != nil {
		return fmt.Errorf("base infra failed to become ready: %w", err)
	}

	log.Printf("base infra ready")

	target := NewTarget("target1", experiment, "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0", map[string]string{})

	log.Printf("starting setup of target %q", target.name)
	if err := target.Setup(ctx, base); err != nil {
		return fmt.Errorf("failed to setup target %q: %w", target.name, err)
	}

	log.Printf("waiting for target %q to be ready", target.name)
	if err := WaitUntil(ctx, target.Ready, 2*time.Second, 30*time.Second); err != nil {
		return fmt.Errorf("target %q failed to become ready: %w", target.name, err)
	}

	log.Printf("target %q ready", target.name)

	return nil
}

func Run(cc *cli.Context) error {
	ctx := cc.Context

	rg := &RunGroup{}

	if commonFlags.prometheusAddr != "" {
		ps, err := NewPrometheusServer(commonFlags.prometheusAddr)
		if err != nil {
			return fmt.Errorf("start prometheus: %w", err)
		}
		rg.Add(Restartable{ps})
	}

	awscfg := aws.NewConfig()
	awscfg.Region = aws.String(commonFlags.awsRegion)
	awscfg.WithHTTPClient(&http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
		Timeout: 10 * time.Second,
	})

	_ = awscfg

	if commonFlags.cpuprofile != "" {
		defer profile.Start(profile.CPUProfile, profile.ProfileFilename(commonFlags.cpuprofile)).Stop()
	}

	if commonFlags.memprofile != "" {
		defer profile.Start(profile.MemProfile, profile.ProfileFilename(commonFlags.memprofile)).Stop()
	}

	return rg.RunAndWait(ctx)
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
