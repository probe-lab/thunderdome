package main

import (
	"fmt"
	"log"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/aws"
	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/exp"
)

var TeardownCommand = &cli.Command{
	Name:   "teardown",
	Usage:  "Teardown an experiment",
	Action: Teardown,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "name",
			Aliases:     []string{"n"},
			Usage:       "Name of experiment",
			Required:    true,
			Destination: &teardownOpts.name,
		},
		&cli.StringFlag{
			Name:        "aws-region",
			Usage:       "AWS region to use when deploying experiments.",
			Value:       "eu-west-1",
			Destination: &commonOpts.awsRegion,
			EnvVars:     []string{envPrefix + "AWS_REGION"},
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Aliases:     []string{"v"},
			Usage:       "Set logging level more verbose to include info level logs",
			Value:       false,
			Destination: &commonOpts.verbose,
			EnvVars:     []string{envPrefix + "VERBOSE"},
		},
		&cli.BoolFlag{
			Name:        "veryverbose",
			Aliases:     []string{"vv"},
			Usage:       "Set logging level very verbose to include debug level logs",
			Value:       false,
			Destination: &commonOpts.veryverbose,
			EnvVars:     []string{envPrefix + "VERY_VERBOSE"},
		},
		&cli.BoolFlag{
			Name:        "nocolor",
			Usage:       "Use plain, machine readable logs",
			Value:       false,
			Destination: &commonOpts.nocolor,
			EnvVars:     []string{envPrefix + "NOCOLOR"},
		},
	},
}

var teardownOpts struct {
	name string
}

func Teardown(cc *cli.Context) error {
	ctx := cc.Context
	setupLogging()

	if commonOpts.awsRegion == "" {
		return fmt.Errorf("aws region must be specified")
	}

	// TODO: lookup experiment
	e := exp.TestExperiment()

	log.Printf("starting tear down of experiment %q", e.Name)
	prov := &aws.Provider{
		Region: commonOpts.awsRegion,
	}

	return prov.Teardown(ctx, e)
}
