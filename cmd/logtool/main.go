package main

import (
	"context"
	"log"
	"os"

	"github.com/pkg/profile"
	"github.com/urfave/cli/v2"
)

const appName = "logtool"

var app = &cli.App{
	Name:        appName,
	Usage:       "a tool for working with gateway logs",
	Description: "logtool is a tool for working with gateway logs",
	Commands: []*cli.Command{
		TailCommand,
	},
	Flags: []cli.Flag{
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
	cpuprofile string
	memprofile string
	awsRegion  string
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	ctx := context.Background()

	if commonOpts.cpuprofile != "" {
		defer profile.Start(profile.CPUProfile, profile.ProfileFilename(commonOpts.cpuprofile)).Stop()
	}

	if commonOpts.memprofile != "" {
		defer profile.Start(profile.MemProfile, profile.ProfileFilename(commonOpts.memprofile)).Stop()
	}

	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}
}
