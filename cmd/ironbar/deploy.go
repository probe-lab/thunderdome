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
	"time"

	"github.com/urfave/cli/v2"
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
	exp := TestExperiment()

	log.Printf("starting deployment of experiment %q", exp.Name)

	base := NewBaseInfra(exp.Name, commonOpts.awsRegion, "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome")
	log.Printf("starting setup of base infra")
	if err := base.Setup(ctx); err != nil {
		return fmt.Errorf("failed to setup base infra: %w", err)
	}

	log.Printf("waiting for base infra to be ready")
	if err := WaitUntil(ctx, base.Ready, 2*time.Second, 30*time.Second); err != nil {
		return fmt.Errorf("base infra failed to become ready: %w", err)
	}

	log.Printf("base infra ready")

	components := make([]Component, 0)

	for _, t := range exp.Targets {
		t := NewTarget(t.Name, exp.Name, base, t.Image, t.Environment)
		components = append(components, t)
	}

	return DeployInParallel(ctx, components)
}
