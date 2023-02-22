package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/thunderdome/cmd/thunderdome/build"
	"github.com/ipfs-shipyard/thunderdome/cmd/thunderdome/exp"
)

var ValidateCommand = &cli.Command{
	Name:      "validate",
	Usage:     "Validate an experiment definition",
	Action:    Validate,
	ArgsUsage: "EXPERIMENT-FILENAME",
	Flags:     commonFlags,
}

var validateOpts struct{}

func Validate(cc *cli.Context) error {
	ctx := cc.Context
	setupLogging()

	if cc.NArg() != 1 {
		return fmt.Errorf("filename experiment must be supplied")
	}

	args := cc.Args()
	e, err := LoadExperiment(ctx, args.Get(0))
	if err != nil {
		return err
	}

	fmt.Printf("Experiment:                  %s\n", e.Name)
	fmt.Printf("Duration:                    %s\n", durationDesc(e.Duration))
	fmt.Printf("Maximum request rate:        %d\n", e.MaxRequestRate)
	fmt.Printf("Maximum concurrent requests: %d\n", e.MaxConcurrency)
	fmt.Printf("Request filter:              %s\n", e.RequestFilter)

	for _, t := range e.Targets {
		fmt.Println()
		fmt.Printf("Target %q\n", t.Name)
		fmt.Printf("  Instance type: %s\n", t.InstanceType)

		if t.Image != "" {
			fmt.Printf("  Image:         %s\n", t.Image)
		} else if t.ImageSpec != nil {
			fmt.Printf("  Image:         %s\n", build.LocalImageName(t.ImageSpec.Hash()))
			if t.ImageSpec.BaseImage != "" {
				fmt.Printf("  Image built:   from base image %s\n", t.ImageSpec.BaseImage)
			} else if t.ImageSpec.Git != nil {
				if t.ImageSpec.Git.Commit != "" {
					fmt.Printf("  Image built:   from commit %s in %s\n", t.ImageSpec.Git.Commit, t.ImageSpec.Git.Repo)
				} else if t.ImageSpec.Git.Tag != "" {
					fmt.Printf("  Image built:   from tag %s in %s\n", t.ImageSpec.Git.Tag, t.ImageSpec.Git.Repo)
				} else if t.ImageSpec.Git.Branch != "" {
					fmt.Printf("  Image built:   from branch %s in %s\n", t.ImageSpec.Git.Branch, t.ImageSpec.Git.Repo)
				} else {
					fmt.Printf("  Image built:   from %s\n", t.ImageSpec.Git.Repo)
				}
			}
			if len(t.ImageSpec.InitCommands) > 0 {
				fmt.Println("  Init commands run when image starts:")
				for _, cmd := range t.ImageSpec.InitCommands {
					fmt.Println("    " + cmd)
				}
			} else {
				fmt.Println("  Init commands run when image starts: none")
			}

		}

		// BaseImage    string
		// Git          *GitSpec
		// InitCommands []string

		if len(t.Environment) == 0 {
			fmt.Println("  Environment:   none")
		} else {
			fmt.Println("  Environment:")
			for k, v := range t.Environment {
				fmt.Printf("    %s=%q\n", k, v)
			}
		}
	}

	return nil
}

func LoadExperiment(ctx context.Context, filename string) (*exp.Experiment, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	e, err := exp.Parse(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("parse experiment definition: %w", err)
	}

	return e, nil
}

func durationDesc(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}
