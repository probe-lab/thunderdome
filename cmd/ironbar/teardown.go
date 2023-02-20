package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slog"

	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/aws"
)

var TeardownCommand = &cli.Command{
	Name:      "teardown",
	Usage:     "Teardown an experiment",
	Action:    Teardown,
	ArgsUsage: "EXPERIMENT-FILENAME",
	Flags:     commonFlags,
}

func Teardown(cc *cli.Context) error {
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
		return fmt.Errorf("failed to read experiment file: %w", err)
	}

	prov, err := aws.NewProvider()
	if err != nil {
		return err
	}
	slog.Info("tearing down experiment " + e.Name)
	return prov.Teardown(ctx, e)
}
