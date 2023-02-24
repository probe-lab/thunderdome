package main

import (
	"fmt"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/thunderdome/cmd/thunderdome/infra"
)

var StatusCommand = &cli.Command{
	Name:   "status",
	Usage:  "Report on the operational status of experiments",
	Action: Status,
	Flags: flags([]cli.Flag{
		&cli.StringFlag{
			Name:        "experiment",
			Aliases:     []string{"e"},
			Usage:       "Name of experiment.",
			Destination: &statusOpts.experiment,
		},
	}),
}

var statusOpts struct {
	experiment string
}

func Status(cc *cli.Context) error {
	ctx := cc.Context
	setupLogging()
	if err := checkEnv(); err != nil {
		return err
	}

	prov, err := infra.NewProvider()
	if err != nil {
		return err
	}

	if statusOpts.experiment != "" {
		out, err := prov.ExperimentStatus(ctx, statusOpts.experiment)
		if err != nil {
			return err
		}

		fmt.Printf("Status       : %s\n", out.Status)
		if out.Stopped.IsZero() {
			fmt.Printf("Running for  : %s\n", time.Since(out.Start).Round(time.Second))
			fmt.Printf("Due to end at: %s\n", out.End.Format(time.Stamp))
		} else {
			fmt.Printf("Ran for      : %s\n", out.Stopped.Sub(out.Start).Round(time.Second))
			fmt.Printf("Stopped at   : %s\n", out.Stopped.Format(time.Stamp))
		}

		dashboard := fmt.Sprintf("https://protocollabs.grafana.net/d/GE2JD7ZVz/experiment-timeline?orgId=1&from=now-1h&to=now&var-experiment=%s", statusOpts.experiment)
		fmt.Println("Grafana dashboard: " + dashboard)

		return nil
	}

	out, err := prov.ListExperiments(ctx)
	if err != nil {
		return err
	}

	if len(out.Items) == 0 {
		fmt.Println("No experiments running or recently stopped")
		return nil
	}

	for _, it := range out.Items {
		if it.Stopped.IsZero() {
			age := time.Since(it.Start).Round(time.Second)
			remaining := it.End.Sub(time.Now()).Round(time.Second)
			fmt.Printf("%-40s %s remaining (running for %s)\n", it.Name, remaining, age)
		} else {
			fmt.Printf("%-40s [stopped]\n", it.Name)
		}
	}

	return nil
}
