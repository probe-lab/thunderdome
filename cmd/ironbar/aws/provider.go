package aws

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/exp"
)

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

type Provider struct {
	Region string
}

func (p *Provider) Deploy(ctx context.Context, e *exp.Experiment) error {
	base := NewBaseInfra(e.Name, p.Region, "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome")
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

	for _, t := range e.Targets {
		t := NewTarget(t.Name, e.Name, base, t.Image, t.Environment)
		components = append(components, t)
	}

	return DeployInParallel(ctx, components)
}

func (p *Provider) Teardown(ctx context.Context, e *exp.Experiment) error {
	base := NewBaseInfra(e.Name, p.Region, "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome")
	if err := base.InspectExisting(ctx); err != nil {
		return fmt.Errorf("failed to inspect existing infra: %w", err)
	}

	components := make([]Component, 0)
	for _, t := range e.Targets {
		t := NewTarget(t.Name, e.Name, base, t.Image, t.Environment)
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

func (p *Provider) Status(ctx context.Context, e *exp.Experiment) error {
	allready := true

	base := NewBaseInfra(e.Name, p.Region, "arn:aws:ecs:eu-west-1:147263665150:cluster/thunderdome")
	ready, err := base.Ready(ctx)
	if err != nil {
		return fmt.Errorf("failed to check base infra ready state: %w", err)
	}
	if ready {
		for _, t := range e.Targets {
			target := NewTarget(t.Name, e.Name, base, t.Image, t.Environment)
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
