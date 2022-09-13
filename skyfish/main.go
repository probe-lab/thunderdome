package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/profile"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

const appName = "skyfish"

var app = &cli.App{
	Name:   appName,
	Action: Run,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:        "rate",
			Usage:       "Number of requests per second to send (if not using an experiment file)",
			Value:       5,
			Destination: &flags.rate,
			EnvVars:     []string{"DEALGOOD_RATE"},
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
	},
}

var flags struct {
	rate           int
	prometheusAddr string
	cpuprofile     string
	memprofile     string
	lokiURI        string
	lokiUsername   string
	lokiPassword   string
	lokiQuery      string
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

	cfg := &LokiConfig{
		URI:      flags.lokiURI,
		Username: flags.lokiUsername,
		Password: flags.lokiPassword,
		Query:    flags.lokiQuery,
	}

	rg := &RunGroup{}

	source, err := NewLokiTailer(cfg, "TODO", flags.rate)
	if err != nil {
		return fmt.Errorf("loki source: %w", err)
	}
	rg.Add(source)

	if flags.prometheusAddr != "" {
		ps, err := NewPrometheusServer(flags.prometheusAddr)
		if err != nil {
			return fmt.Errorf("start prometheus: %w", err)
		}
		rg.Add(Restartable{ps})
	}

	publisher, err := NewPublisher(source.Chan())
	if err != nil {
		return fmt.Errorf("new publisher: %w", err)
	}
	rg.Add(publisher)

	if flags.cpuprofile != "" {
		defer profile.Start(profile.CPUProfile, profile.ProfileFilename(flags.cpuprofile)).Stop()
	}

	if flags.memprofile != "" {
		defer profile.Start(profile.MemProfile, profile.ProfileFilename(flags.memprofile)).Stop()
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
