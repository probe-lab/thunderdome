package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/thunderdome/cmd/thunderdome/aws"
)

var DeployCommand = &cli.Command{
	Name:      "deploy",
	Usage:     "Deploy an experiment",
	Action:    Deploy,
	ArgsUsage: "EXPERIMENT-FILENAME",
	Flags:     commonFlags,
}

type deployOpts struct{}

func Deploy(cc *cli.Context) error {
	ctx := cc.Context
	setupLogging()
	if err := checkBuildEnv(); err != nil {
		return err
	}

	if cc.NArg() != 1 {
		return fmt.Errorf("filename experiment must be supplied")
	}

	args := cc.Args()
	e, err := LoadExperiment(ctx, args.Get(0))
	if err != nil {
		return err
	}

	prov, err := aws.NewProvider()
	if err != nil {
		return err
	}

	return prov.Deploy(ctx, e)
}
