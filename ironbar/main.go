package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

const appName = "ironbar"

var app = &cli.App{
	Name:        appName,
	Usage:       "a tool for managing experiments",
	Description: "ironbar is a tool for managing experiments",
	Commands: []*cli.Command{
		DeployCommand,
		TeardownCommand,
		StatusCommand,
		ImageCommand,
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "prometheus-addr",
			Usage:       "Network address to start a prometheus metric exporter server on (example: :9991)",
			Value:       "",
			Destination: &commonOpts.prometheusAddr,
			EnvVars:     []string{"IRONBAR_PROMETHEUS_ADDR"},
		},
		&cli.StringFlag{
			Name:        "cpuprofile",
			Usage:       "Write a CPU profile to the specified file before exiting.",
			Value:       "",
			Destination: &commonOpts.cpuprofile,
			EnvVars:     []string{"IRONBAR_CPUPROFILE"},
		},
		&cli.StringFlag{
			Name:        "memprofile",
			Usage:       "Write an allocation profile to the file before exiting.",
			Value:       "",
			Destination: &commonOpts.memprofile,
			EnvVars:     []string{"IRONBAR_MEMPROFILE"},
		},
	},
}

var commonOpts struct {
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
