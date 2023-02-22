package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slog"

	"github.com/ipfs-shipyard/thunderdome/pkg/prom"
	"github.com/ipfs-shipyard/thunderdome/pkg/run"
)

func main() {
	ctx := context.Background()
	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

var options struct {
	addr                 string
	verbose              bool
	veryverbose          bool
	diagnosticsAddr      string
	awsRegion            string
	experimentsTableName string
	monitorInterval      int
	settle               int
}

const (
	appName   = "ironbar"
	envPrefix = "IRONBAR_"
)

var app = &cli.App{
	Name:        appName,
	HelpName:    appName,
	Description: "ironbar is a service for managing experiments",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "addr",
			Aliases:     []string{"a"},
			Usage:       "Listen for traces on `ADDRESS:PORT`",
			Value:       ":8321",
			EnvVars:     []string{envPrefix + "ADDR"},
			Destination: &options.addr,
		},
		&cli.StringFlag{
			Name:        "diag-addr",
			Aliases:     []string{"da"},
			Usage:       "Run diagnostics server for metrics on `ADDRESS:PORT`",
			Value:       "",
			EnvVars:     []string{envPrefix + "DIAG_ADDR"},
			Destination: &options.diagnosticsAddr,
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Aliases:     []string{"v"},
			Usage:       "Set logging level more verbose to include info level logs",
			Value:       true,
			EnvVars:     []string{envPrefix + "VERBOSE"},
			Destination: &options.verbose,
		},
		&cli.BoolFlag{
			Name:        "veryverbose",
			Aliases:     []string{"vv"},
			Usage:       "Set logging level very verbose to include debug level logs",
			Value:       false,
			EnvVars:     []string{envPrefix + "VERY_VERBOSE"},
			Destination: &options.veryverbose,
		},
		&cli.StringFlag{
			Name:        "experiments-table-name",
			Aliases:     []string{"t"},
			Usage:       "The name of the experiments table.",
			Value:       "",
			EnvVars:     []string{envPrefix + "EXPERIMENTS_TABLE_NAME"},
			Destination: &options.experimentsTableName,
		},
		&cli.StringFlag{
			Name:        "aws-region",
			Aliases:     []string{"r"},
			Usage:       "The name of the AWS region ironbar is running in.",
			Value:       "",
			EnvVars:     []string{envPrefix + "AWS_REGION"},
			Destination: &options.awsRegion,
		},
		&cli.IntFlag{
			Name:        "monitor-interval",
			Usage:       "The number of minutes to wait between checks on experiment resources.",
			Value:       1,
			EnvVars:     []string{envPrefix + "MONITOR_INTERVAL"},
			Destination: &options.monitorInterval,
		},
		&cli.IntFlag{
			Name:        "settle",
			Usage:       "The number of minutes to wait for resources to settle before beginning checks.",
			Value:       5,
			EnvVars:     []string{envPrefix + "SETTLE"},
			Destination: &options.settle,
		},
	},
	Action:          Run,
	HideHelpCommand: true,
}

func Run(cc *cli.Context) error {
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelWarn)
	slog.SetDefault(slog.New(slog.HandlerOptions{Level: logLevel}.NewTextHandler(os.Stdout)))

	if options.verbose {
		logLevel.Set(slog.LevelInfo)
	}
	if options.veryverbose {
		logLevel.Set(slog.LevelDebug)
	}

	ctx, cancel := context.WithCancel(cc.Context)
	defer cancel()

	rg := new(run.Group)

	// Init metric reporting if required
	if options.diagnosticsAddr != "" {
		ps, err := prom.NewPrometheusServer(options.diagnosticsAddr, appName)
		if err != nil {
			return fmt.Errorf("start prometheus: %w", err)
		}
		rg.Add(ps)
	}

	db := &DB{
		AwsRegion: options.awsRegion,
		TableName: options.experimentsTableName,
	}

	svr := NewServer(
		ctx,
		db,
		options.awsRegion,
		time.Duration(options.monitorInterval)*time.Minute,
		time.Duration(options.settle)*time.Minute,
	)
	rg.Add(svr)

	return rg.RunAndWait(ctx)
}

type DiagRunner struct {
	addr string
}

func (dr *DiagRunner) Run(ctx context.Context) error {
	diagListener, err := net.Listen("tcp", dr.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %q: %w", dr.addr, err)
	}

	pe, err := RegisterPrometheusExporter("tracecatcher")
	if err != nil {
		return fmt.Errorf("failed to register prometheus exporter: %w", err)
	}

	mx := mux.NewRouter()
	mx.Handle("/metrics", pe)

	srv := &http.Server{
		Handler:     mx,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			slog.Error("failed to shut down diagnostics server", err)
		}
	}()

	slog.Info("starting diagnostics server", "addr", dr.addr)
	return srv.Serve(diagListener)
}
