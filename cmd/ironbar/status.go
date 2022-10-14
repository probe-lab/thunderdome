package main

import (
	"fmt"
	"log"

	"github.com/urfave/cli/v2"
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
	exp := TestExperiment()

	log.Printf("checking status of experiment %q", exp.Name)

	allready := true

	base := NewBaseInfra(exp.Name, commonOpts.awsRegion, "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome")
	ready, err := base.Ready(ctx)
	if err != nil {
		return fmt.Errorf("failed to check base infra ready state: %w", err)
	}
	if ready {
		for _, t := range exp.Targets {
			target := NewTarget(t.Name, exp.Name, base, t.Image, t.Environment)
			ready, err := target.Ready(ctx)
			if err != nil {
				return fmt.Errorf("failed to check target %q ready state: %w", t.Name, err)
			}
			if ready {
				log.Printf("target %q ready", t.Name)
			} else {
				allready = false
			}
		}
	} else {
		allready = false
	}

	if allready {
		log.Printf("all components ready")
	}
	return nil
}
