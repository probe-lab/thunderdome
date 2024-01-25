package main

import (
	"fmt"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/probe-lab/thunderdome/cmd/thunderdome/infra"
)

var DeployCommand = &cli.Command{
	Name:      "deploy",
	Usage:     "Deploy an experiment",
	Action:    Deploy,
	ArgsUsage: "EXPERIMENT-FILENAME",
	Flags: flags(
		[]cli.Flag{
			&cli.IntFlag{
				Name:        "duration",
				Required:    true,
				Aliases:     []string{"d"},
				Usage:       "Duration to run the experiment for, in minutes.",
				Destination: &deployOpts.duration,
			},
			&cli.BoolFlag{
				Name:        "force",
				Required:    false,
				Aliases:     []string{"f"},
				Usage:       "Force docker images to be rebuilt.",
				Destination: &deployOpts.forceBuild,
			},
		},
	),
}

var deployOpts struct {
	duration   int
	forceBuild bool
}

func Deploy(cc *cli.Context) error {
	ctx := cc.Context
	setupLogging()
	if err := checkBuildEnv(); err != nil {
		return err
	}

	if deployOpts.duration < 5 {
		return fmt.Errorf("duration must be at least 5 minutes")
	}

	if cc.NArg() != 1 {
		return fmt.Errorf("filename experiment must be supplied")
	}

	args := cc.Args()
	e, err := LoadExperiment(ctx, args.Get(0))
	if err != nil {
		return err
	}
	e.Duration = time.Duration(deployOpts.duration) * time.Minute

	prov, err := infra.NewProvider()
	if err != nil {
		return err
	}

	return prov.Deploy(ctx, e, deployOpts.forceBuild)
}
