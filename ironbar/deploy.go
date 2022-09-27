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
	},
}

func Deploy(cc *cli.Context) error {
	ctx := cc.Context

	experiment := "ironbar_test"

	log.Printf("starting deployment of experiment %q", experiment)

	base := NewBaseInfra(experiment, "eu-west-1", "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome")
	log.Printf("starting setup of base infra")
	if err := base.Setup(ctx); err != nil {
		return fmt.Errorf("failed to setup base infra: %w", err)
	}

	log.Printf("waiting for base infra to be ready")
	if err := WaitUntil(ctx, base.Ready, 2*time.Second, 30*time.Second); err != nil {
		return fmt.Errorf("base infra failed to become ready: %w", err)
	}

	log.Printf("base infra ready")

	// target := NewTarget("target1", experiment, "147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-v0.15.0", map[string]string{})

	// log.Printf("starting setup of target %q", target.name)
	// if err := target.Setup(ctx, base); err != nil {
	// 	return fmt.Errorf("failed to setup target %q: %w", target.name, err)
	// }

	// log.Printf("waiting for target %q to be ready", target.name)
	// if err := WaitUntil(ctx, target.Ready, 2*time.Second, 30*time.Second); err != nil {
	// 	return fmt.Errorf("target %q failed to become ready: %w", target.name, err)
	// }

	// log.Printf("target %q ready", target.name)

	return nil
}
