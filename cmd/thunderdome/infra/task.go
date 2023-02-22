package infra

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
)

type Check struct {
	Name        string
	Func        func(context.Context, *session.Session) (bool, error)
	FailureText string // optional text to use to describe failure
}

func CheckSequence(ctx context.Context, sess *session.Session, component string, checks ...Check) (bool, error) {
	logger := slog.With("component", component)

	for _, c := range checks {
		ready, err := c.Func(ctx, sess)
		if err != nil {
			logger.Error(c.Name, err)
			return false, err
		}
		if !ready {
			failureText := c.FailureText
			if failureText == "" {
				failureText = fmt.Sprintf("%s: no", c.Name)
			}
			logger.Log(slog.LevelError, failureText)
			return false, nil
		}
		logger.Info(c.Name)
	}

	return true, nil
}

type Task struct {
	Name  string
	Check Check
	Func  func(context.Context, *session.Session) error
}

// ExecuteTask executes a task. It first runs the task's Check function, if any. If this returns false then
// the task Func is called. Then the Check function is polled until it returns true.
func ExecuteTask(ctx context.Context, sess *session.Session, component string, task Task) error {
	logger := slog.With("component", component, "step", task.Name)

	// Do a pre-check to see if the correct state already exists
	if task.Check.Func != nil {
		logger.Info(fmt.Sprintf("checking whether %s", task.Check.Name))
		ok, err := task.Check.Func(ctx, sess)
		if err != nil {
			return fmt.Errorf("failed precondition check (%s:%s): %w", component, task.Check.Name, err)
		}
		if ok {
			logger.Info(task.Check.Name)
			return nil
		}
		failureText := task.Check.FailureText
		if failureText == "" {
			failureText = fmt.Sprintf("%s: no", task.Check.Name)
		}
		logger.Info(failureText)
	}

	logger.Info("executing step")
	err := task.Func(ctx, sess)
	if err != nil {
		logger.Error("failed", err)
		return fmt.Errorf("failed task (%s): %w", task.Name, err)
	}

	if task.Check.Func != nil {
		if err := WaitUntilCheck(ctx, sess, logger, task.Check, 2*time.Second, 30*time.Second); err != nil {
			logger.Error("failed", err)
			return fmt.Errorf("failed check (%s: %s): %w", component, task.Check.Name, err)
		}
		logger.Info(task.Check.Name)
	}

	return nil
}

func TaskSequence(ctx context.Context, sess *session.Session, component string, tasks ...Task) error {
	for _, task := range tasks {
		if err := ExecuteTask(ctx, sess, component, task); err != nil {
			return err
		}
	}

	return nil
}

type Component interface {
	Name() string
	Setup(ctx context.Context) error
	Ready(ctx context.Context) (bool, error)
	Teardown(context.Context) error
}

func DeployInParallel(ctx context.Context, comps []Component) error {
	g, ctx := errgroup.WithContext(ctx)

	for _, c := range comps {
		c := c // ugh, hurry up https://github.com/golang/go/discussions/56010
		g.Go(func() error {
			if err := c.Setup(ctx); err != nil {
				return fmt.Errorf("%s failed to setup: %w", c.Name(), err)
			}
			if err := WaitUntil(ctx, slog.With("component", c.Name()), "is ready", c.Ready, 2*time.Second, 30*time.Second); err != nil {
				return fmt.Errorf("%s failed to become ready: %w", c.Name(), err)
			}
			return nil
		})
	}
	// Wait for all deployments to run to completion.
	if err := g.Wait(); err != nil {
		if !errors.Is(err, context.Canceled) {
			return err
		}
	}

	return nil
}

func TeardownInParallel(ctx context.Context, comps []Component) error {
	g, ctx := errgroup.WithContext(ctx)

	for _, c := range comps {
		c := c // ugh, hurry up https://github.com/golang/go/discussions/56010
		g.Go(func() error {
			if err := c.Teardown(ctx); err != nil {
				return fmt.Errorf("%s failed to tear down: %w", c.Name(), err)
			}
			return nil
		})
	}
	// Wait for all deployments to run to completion.
	if err := g.Wait(); err != nil {
		if !errors.Is(err, context.Canceled) {
			return err
		}
	}

	return nil
}
