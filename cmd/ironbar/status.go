package main

import (
	"fmt"
	"log"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/aws"
	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/exp"
)

var StatusCommand = &cli.Command{
	Name:   "status",
	Usage:  "Report on the operational status of an experiment",
	Action: Status,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "name",
			Aliases:     []string{"n"},
			Usage:       "Name of experiment",
			Required:    true,
			Destination: &statusOpts.name,
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

var statusOpts struct {
	name string
}

func Status(cc *cli.Context) error {
	ctx := cc.Context
	if commonOpts.awsRegion == "" {
		return fmt.Errorf("aws region must be specified")
	}

	// TODO: fetch experiment from storage
	e := exp.TestExperiment()

	log.Printf("checking status of experiment %q", e.Name)
	prov := &aws.Provider{
		Region: commonOpts.awsRegion,
	}

	return prov.Status(ctx, e)
}
