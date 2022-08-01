package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/urfave/cli/v2"
)

const appName = "dealgood"

var app = &cli.App{
	Name:   appName,
	Action: Run,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:        "nogui",
			Usage:       "Disable GUI",
			Value:       true,
			Destination: &flags.nogui,
			EnvVars:     []string{"DEALGOOD_NOGUI"},
		},
	},
}

var flags struct {
	nogui bool
}

func main() {
	ctx := context.Background()
	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func Run(cc *cli.Context) error {
	source := NewRandomRequestSource(sampleRequests)
	backends := []*Backend{
		{
			Name:    "local",
			BaseURL: "http://localhost:8080",
		},
	}

	if flags.nogui {
		return nogui(cc.Context, source, backends)
	}

	g, err := NewGui(source, backends)
	if err != nil {
		return fmt.Errorf("gui: %w", err)
	}
	defer g.Close()
	return g.Show(cc.Context, 100*time.Millisecond)
}

func nogui(ctx context.Context, source RequestSource, backends []*Backend) error {
	timings := make(chan *RequestTiming, 10000)

	coll := NewCollector(timings, 100*time.Millisecond)
	go coll.Run(ctx)

	l := &Loader{
		Source:      source,
		Backends:    backends,
		Rate:        1000, // per second
		Concurrency: 50,   // concurrent requests per backend
		Duration:    60 * time.Second,
		Timings:     timings,
	}

	if err := l.Send(ctx); err != nil {
		log.Printf("loader error: %v", err)
	}
	close(timings)
	return nil
}
