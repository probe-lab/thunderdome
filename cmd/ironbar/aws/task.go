package aws

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"golang.org/x/sync/errgroup"
)

type Check struct {
	Name        string
	Func        func(context.Context, *session.Session) (bool, error)
	FailureText string // optional text to use to describe failure
}

func CheckSequence(ctx context.Context, sess *session.Session, prefix string, checks ...Check) (bool, error) {
	if prefix != "" {
		prefix += ": "
	}
	for _, c := range checks {
		ready, err := c.Func(ctx, sess)
		if err != nil {
			log.Printf("%s%s: error %v", prefix, c.Name, err)
			return false, err
		}
		if !ready {
			failureText := c.FailureText
			if failureText == "" {
				failureText = fmt.Sprintf("%s: no", c.Name)
			}
			log.Printf("%s%s", prefix, failureText)
			return false, nil
		}
		log.Printf("%s%s", prefix, c.Name)
	}

	return true, nil
}

type Task struct {
	Name  string
	Check Check
	Func  func(context.Context, *session.Session) error
}

func ExecuteTask(ctx context.Context, sess *session.Session, prefix string, task Task) error {
	tag := task.Name
	if prefix != "" {
		tag = prefix + ": " + tag
	}

	// Do a pre-check to see if the correct state already exists
	if task.Check.Func != nil {
		log.Printf("%s: checking whether %s", tag, task.Check.Name)
		ok, err := task.Check.Func(ctx, sess)
		if err != nil {
			return fmt.Errorf("failed precondition check (%s%s): %w", tag, task.Check.Name, err)
		}
		if ok {
			log.Printf("%s: %s", tag, task.Check.Name)
			return nil
		}
		failureText := task.Check.FailureText
		if failureText == "" {
			failureText = fmt.Sprintf("%s: no", task.Check.Name)
		}
		log.Printf("%s: %s", tag, failureText)
	}

	log.Printf("%s: executing task", tag)
	err := task.Func(ctx, sess)
	if err != nil {
		log.Printf("%s: error %v", tag, err)
		return fmt.Errorf("failed task (%s): %w", tag, err)
	}

	if task.Check.Func != nil {
		if err := WaitUntilCheck(ctx, sess, tag, task.Check, 2*time.Second, 30*time.Second); err != nil {
			log.Printf("%s: %s: error %v", tag, task.Check.Name, err)
			return fmt.Errorf("failed check (%s: %s): %w", tag, task.Check.Name, err)
		}
		log.Printf("%s: %s", tag, task.Check.Name)
	}

	return nil
}

func TaskSequence(ctx context.Context, sess *session.Session, prefix string, tasks ...Task) error {
	for _, task := range tasks {
		if err := ExecuteTask(ctx, sess, prefix, task); err != nil {
			return err
		}
	}

	return nil
}

func WaitUntilCheck(ctx context.Context, sess *session.Session, prefix string, check Check, delay time.Duration, interval time.Duration) error {
	if check.Func == nil {
		return nil
	}
	first := true
	if delay > 0 {
		log.Printf("%s: waiting until %s", prefix, check.Name)
		time.Sleep(delay)
		first = false
	}
	done, err := check.Func(ctx, sess)
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	tick := time.NewTicker(interval)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			done, err := check.Func(ctx, sess)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			if first {
				first = false
				log.Printf("%s: waiting until %s", prefix, check.Name)
			} else {
				log.Printf("%s: still waiting until %s", prefix, check.Name)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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
			log.Printf("%s: deploying", c.Name())
			if err := c.Setup(ctx); err != nil {
				return fmt.Errorf("%s failed to setup: %w", c.Name(), err)
			}
			log.Printf("%s: waiting to become ready", c.Name())
			if err := WaitUntil(ctx, c.Ready, 2*time.Second, 30*time.Second); err != nil {
				return fmt.Errorf("%s failed to become ready: %w", c.Name(), err)
			}
			log.Printf("%s: ready", c.Name())
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
			log.Printf("%s: tearing down", c.Name())
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
