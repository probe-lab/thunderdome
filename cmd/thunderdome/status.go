package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/thunderdome/cmd/thunderdome/infra"
)

var StatusCommand = &cli.Command{
	Name:      "status",
	Usage:     "Report on the operational status of an experiment",
	Action:    Status,
	ArgsUsage: "EXPERIMENT-FILENAME",
	Flags:     commonFlags,
}

func Status(cc *cli.Context) error {
	ctx := cc.Context
	setupLogging()
	if err := checkEnv(); err != nil {
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
	return prov.Status(ctx, e)
}
