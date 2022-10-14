package main

// Resources needed
// resource "aws_ecs_service" "target" {
// resource "aws_service_discovery_service" "target" {
// resource "aws_ecs_task_definition" "target" {
// + resource "aws_iam_role" "experiment" {
// + resource "aws_iam_role_policy_attachment" "experiment-ssm" {
// resource "aws_sqs_queue" "requests" {
// resource "aws_sqs_queue_policy" "requests" {
// resource "aws_sns_topic_subscription" "requests_sqs_target" {
// resource "aws_ecs_service" "dealgood" {
// resource "aws_ecs_task_definition" "dealgood" {

import (
	"fmt"
	"log"

	"github.com/urfave/cli/v2"
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
			EnvVars:     []string{"IRONBAR_AWS_REGION"},
		},
	},
}

var teardownOpts struct {
	name string
}

func Teardown(cc *cli.Context) error {
	ctx := cc.Context
	if commonOpts.awsRegion == "" {
		return fmt.Errorf("aws region must be specified")
	}

	// TODO: lookup experiment
	exp := TestExperiment()

	log.Printf("starting tear down of experiment %q", exp.Name)

	base := NewBaseInfra(exp.Name, commonOpts.awsRegion, "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome")
	if err := base.InspectExisting(ctx); err != nil {
		return fmt.Errorf("failed to inspect existing infra: %w", err)
	}

	components := make([]Component, 0)
	for _, t := range exp.Targets {
		t := NewTarget(t.Name, exp.Name, base, t.Image, t.Environment)
		components = append(components, t)
	}
	if err := TeardownInParallel(ctx, components); err != nil {
		return err
	}

	if err := base.Teardown(ctx); err != nil {
		return fmt.Errorf("base infra: failed to tear down: %w", err)
	}

	return nil
}
