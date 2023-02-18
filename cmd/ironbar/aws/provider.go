package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/exp"
	"github.com/kortschak/utter"
	"golang.org/x/exp/slog"
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
	base := NewBaseInfra(e.Name, p.Region)
	if err := base.Setup(ctx); err != nil {
		return fmt.Errorf("failed to setup base infra: %w", err)
	}

	if err := WaitUntil(ctx, slog.With("component", base.Name()), "is ready", base.Ready, 2*time.Second, 30*time.Second); err != nil {
		return fmt.Errorf("base infra failed to become ready: %w", err)
	}

	components := make([]Component, 0, len(e.Targets))
	targets := make([]*Target, 0, len(e.Targets))
	for _, t := range e.Targets {
		t := NewTarget(t.Name, e.Name, base, t.Image, t.Environment)
		targets = append(targets, t)
		components = append(components, t)
	}
	if err := DeployInParallel(ctx, components); err != nil {
		return fmt.Errorf("targets failed to deploy: %w", err)
	}

	targetURLs := make([]string, len(targets))
	for i := range targets {
		targetURLs[i] = targets[i].GatewayURL()
	}

	d := NewDealgood(e.Name, base, e.Dealgood.Image, e.Dealgood.Environment, targetURLs)
	if err := d.Setup(ctx); err != nil {
		return fmt.Errorf("failed to setup dealgood: %w", err)
	}

	if err := WaitUntil(ctx, slog.With("component", d.Name()), "is ready", d.Ready, 2*time.Second, 30*time.Second); err != nil {
		return fmt.Errorf("dealgood failed to become ready: %w", err)
	}

	return nil
}

func (p *Provider) Teardown(ctx context.Context, e *exp.Experiment) error {
	base := NewBaseInfra(e.Name, p.Region)
	if err := base.InspectExisting(ctx); err != nil {
		return fmt.Errorf("failed to inspect existing infra: %w", err)
	}

	d := NewDealgood(e.Name, base, e.Dealgood.Image, e.Dealgood.Environment, []string{})
	if err := d.Teardown(ctx); err != nil {
		return fmt.Errorf("failed to teardown dealgood: %w", err)
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
		return fmt.Errorf("failed to tear down base infra: %w", err)
	}

	return nil
}

func (p *Provider) Status(ctx context.Context, e *exp.Experiment) error {
	allready := true

	base := NewBaseInfra(e.Name, p.Region)
	ready, err := base.Ready(ctx)
	if err != nil {
		return fmt.Errorf("failed to check %s ready state: %w", base.Name(), err)
	}
	if ready {
		for _, t := range e.Targets {
			target := NewTarget(t.Name, e.Name, base, t.Image, t.Environment)
			ready, err := target.Ready(ctx)
			if err != nil {
				return fmt.Errorf("failed to check %s ready state: %w", target.Name(), err)
			}
			if ready {
				slog.Info("ready", "component", target.Name())
			} else {
				allready = false
			}
		}

		d := NewDealgood(e.Name, base, e.Dealgood.Image, e.Dealgood.Environment, []string{})
		ready, err := d.Ready(ctx)
		if err != nil {
			return fmt.Errorf("failed to check %s ready state: %w", d.Name(), err)
		}
		if ready {
			slog.Info("ready", "component", d.Name())
		} else {
			allready = false
		}
	} else {
		allready = false
	}

	if allready {
		slog.Info("all components ready")
	}
	return nil
}

func debugf(t string, args ...any) {
	slog.Debug(fmt.Sprintf(t, args...))
}

func warnf(t string, args ...any) {
	slog.Warn(fmt.Sprintf(t, args...))
}

func errorf(t string, args ...any) {
	slog.Log(slog.LevelError, fmt.Sprintf(t, args...))
}

func dump(vs ...any) {
	for _, v := range vs {
		fmt.Println(utter.Sdump(v))
	}
}
