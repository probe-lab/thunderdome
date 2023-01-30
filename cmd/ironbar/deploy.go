package main

import (
	"fmt"
	"log"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/aws"
	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/exp"
)

var DeployCommand = &cli.Command{
	Name:   "deploy",
	Usage:  "Deploy an experiment",
	Action: Deploy,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "experiment",
			Aliases: []string{"x"},
			Usage:   "Path to experiment definition",
		},
		&cli.StringFlag{
			Name:        "aws-region",
			Usage:       "AWS region to use when deploying experiments.",
			Value:       "eu-west-1",
			Destination: &commonOpts.awsRegion,
			EnvVars:     []string{"IRONBAR_AWS_REGION"},
		},
	},
}

type deployOpts struct{}

func Deploy(cc *cli.Context) error {
	ctx := cc.Context

	if commonOpts.awsRegion == "" {
		return fmt.Errorf("aws region must be specified")
	}

	// TODO: lookup experiment
	e := exp.TestExperiment()

	log.Printf("starting deployment of experiment %q", e.Name)

	prov := &aws.Provider{
		Region: commonOpts.awsRegion,
	}

	return prov.Deploy(ctx, e)
}
