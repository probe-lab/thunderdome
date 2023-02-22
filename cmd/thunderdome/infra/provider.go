package infra

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ipfs-shipyard/thunderdome/cmd/thunderdome/build"
	"github.com/ipfs-shipyard/thunderdome/cmd/thunderdome/exp"
	"github.com/kortschak/utter"
	"golang.org/x/exp/slog"

	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/api"
)

type Provider struct {
	region     string
	imageCache map[string]string
}

func NewProvider() (*Provider, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		return nil, fmt.Errorf("Environment variable AWS_REGION should be set to the region Thunderdome is running in")
	}
	return &Provider{
		region: region,
	}, nil
}

func (p *Provider) Deploy(ctx context.Context, e *exp.Experiment) error {
	base, err := NewBaseInfra(p.region)
	if err != nil {
		return fmt.Errorf("failed to read base infra: %w", err)
	}
	if err := base.Verify(ctx); err != nil {
		return fmt.Errorf("failed to verify base infra: %w", err)
	}

	// Build all the images
	for _, t := range e.Targets {
		if t.Image != "" {
			continue
		}

		if t.ImageSpec == nil {
			return fmt.Errorf("no image found for target %s", t.Name)
		}

		var err error
		slog.Info("building docker image", "component", "target "+t.Name)
		t.Image, err = p.BuildImage(ctx, t.ImageSpec, base.EcrBaseURL)
		slog.Debug("using docker image", "component", "target "+t.Name, "image", t.Image)
		if err != nil {
			return fmt.Errorf("failed to build image for target %s", t.Name)
		}
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

	d := NewDealgood(e.Name, base).
		WithTargetURLs(targetURLs).
		WithMaxRequestRate(e.MaxRequestRate).
		WithMaxConcurrency(e.MaxConcurrency).
		WithRequestFilter(e.RequestFilter)

	if err := d.Setup(ctx); err != nil {
		return fmt.Errorf("failed to setup dealgood: %w", err)
	}

	if err := WaitUntil(ctx, slog.With("component", d.Name()), "is ready", d.Ready, 2*time.Second, 30*time.Second); err != nil {
		return fmt.Errorf("dealgood failed to become ready: %w", err)
	}

	var res []api.Resource
	res = append(res, d.Resources()...)
	for i := range targets {
		res = append(res, targets[i].Resources()...)
	}

	if err := WaitUntil(ctx, slog.With(), "experiment registered", RegisterExperiment(base.IronbarAddr, e, res), 2*time.Second, 30*time.Second); err != nil {
		return fmt.Errorf("failed to register experiment: %w", err)
	}

	return nil
}

func (p *Provider) Teardown(ctx context.Context, e *exp.Experiment) error {
	base, err := NewBaseInfra(p.region)
	if err != nil {
		return fmt.Errorf("failed to read base infra: %w", err)
	}
	if err := base.Verify(ctx); err != nil {
		return fmt.Errorf("failed to verify base infra: %w", err)
	}

	d := NewDealgood(e.Name, base)
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

	return nil
}

func (p *Provider) Status(ctx context.Context, e *exp.Experiment) error {
	allready := true

	base, err := NewBaseInfra(p.region)
	if err != nil {
		return fmt.Errorf("failed to read base infra: %w", err)
	}
	if err := base.Verify(ctx); err != nil {
		return fmt.Errorf("failed to verify base infra: %w", err)
	}
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

	d := NewDealgood(e.Name, base)
	ready, err := d.Ready(ctx)
	if err != nil {
		return fmt.Errorf("failed to check %s ready state: %w", d.Name(), err)
	}
	if ready {
		slog.Info("ready", "component", d.Name())
	} else {
		allready = false
	}

	if allready {
		slog.Info("all components ready")
	}
	return nil
}

func (p *Provider) BuildImage(ctx context.Context, is *exp.ImageSpec, ecrBaseURL string) (string, error) {
	tag := is.Hash()
	image, ok := p.imageCache[tag]
	if ok {
		return image, nil
	}

	_, err := build.Build(ctx, tag, is)
	if err != nil {
		return "", fmt.Errorf("build image: %w", err)
	}

	remoteImage, err := build.PushImage(tag, p.region, ecrBaseURL)
	if err != nil {
		return "", fmt.Errorf("push image: %w", err)
	}

	if p.imageCache == nil {
		p.imageCache = make(map[string]string)
	}
	p.imageCache[tag] = remoteImage
	return remoteImage, err
}

func (p *Provider) ExperimentStatus(ctx context.Context, name string) (*api.ExperimentStatusOutput, error) {
	base, err := NewBaseInfra(p.region)
	if err != nil {
		return nil, fmt.Errorf("failed to read base infra: %w", err)
	}

	out, err := GetExperimentStatus(ctx, base.IronbarAddr, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	return out, nil
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
