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
	},
}

var statusOpts struct {
	name string
}

func Status(cc *cli.Context) error {
	ctx := cc.Context

	experiment := statusOpts.name

	log.Printf("checking status of experiment %q", experiment)

	base := NewBaseInfra(experiment, "eu-west-1", "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome")

	log.Printf("checking base infrastructure status")
	statuses, err := base.Status(ctx)
	if err != nil {
		return fmt.Errorf("base infra status: %w", err)
	}

	baseReady := true
	for _, st := range statuses {
		if st.Error != nil {
			log.Printf("base infra component %q gave error: %v", st.Name, st.Error)
			baseReady = false
			continue
		}
		if st.Ready {
			log.Printf("base infra component %q ready", st.Name)
		} else {
			baseReady = false
			log.Printf("base infra component %q does not exist or is not ready", st.Name)
		}
	}

	if !baseReady {
		return nil
	}
	log.Printf("base infra ready")

	// target := NewTarget("target1", experiment, "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0", map[string]string{})

	// log.Printf("checking if %q is ready", target.name)
	// if err := WaitUntil(ctx, target.Ready, 2*time.Second, 30*time.Second); err != nil {
	// 	return fmt.Errorf("target %q failed to become ready: %w", target.name, err)
	// }
	// log.Printf("target %q ready", target.name)

	return nil
}

type ComponentStatus struct {
	Name  string
	Ready bool
	Error error
}
