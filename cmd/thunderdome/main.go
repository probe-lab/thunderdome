package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slog"
)

const (
	appName   = "thunderdome"
	envPrefix = "THUNDERDOME_"
)

var app = &cli.App{
	Name:        appName,
	Usage:       "a tool for managing experiments",
	Description: "thunderdome is a tool for managing experiments",
	Commands: []*cli.Command{
		DeployCommand,
		TeardownCommand,
		StatusCommand,
		ImageCommand,
		ValidateCommand,
	},
	Flags: commonFlags,
}

func main() {
	ctx := context.Background()
	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func setupLogging() {
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelWarn)
	if commonOpts.verbose {
		logLevel.Set(slog.LevelInfo)
	}
	if commonOpts.veryverbose {
		logLevel.Set(slog.LevelDebug)
	}

	if commonOpts.nocolor {
		slog.SetDefault(slog.New(slog.HandlerOptions{Level: logLevel}.NewTextHandler(os.Stdout)))
	} else {
		h := NewInteractiveHandler()
		h = h.WithLevel(logLevel.Level())
		slog.SetDefault(slog.New(h))
	}
}

func checkBuildEnv() error {
	if os.Getenv("AWS_PROFILE") == "" {
		return fmt.Errorf("Environment variable AWS_PROFILE should be set to a valid AWS profile name to allow pushing of images to ECR")
	}

	return checkEnv()
}

func checkEnv() error {
	if os.Getenv("AWS_REGION") == "" {
		return fmt.Errorf("Environment variable AWS_REGION should be set to the region Thunderdome is running in")
	}

	return nil
}

var commonOpts struct {
	verbose     bool
	veryverbose bool
	nocolor     bool
}

var commonFlags = []cli.Flag{
	&cli.BoolFlag{
		Name:        "verbose",
		Aliases:     []string{"v"},
		Usage:       "Set logging level more verbose to include info level logs",
		Value:       true,
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
}

func flags(fs []cli.Flag) []cli.Flag {
	fs = append(fs, commonFlags...)
	return fs
}
