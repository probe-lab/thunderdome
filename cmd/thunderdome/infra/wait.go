package infra

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"golang.org/x/exp/slog"
)

// WaitUntil repeatedly calls condition until condition returns true or an error or the context is cancelled.
// delay is the time to wait before calling condition for the first time.
// interval is the time to wait between each subsequent call to condition
func WaitUntil(ctx context.Context, logger *slog.Logger, goal string, condition func(context.Context) (bool, error), delay time.Duration, interval time.Duration) error {
	first := true
	if delay > 0 {
		logger.Info("waiting until " + goal)
		time.Sleep(delay)
		first = false
	}
	done, err := condition(ctx)
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
			done, err := condition(ctx)
			if err != nil {
				return err
			}
			if done {
				logger.Info(goal)
				return nil
			}
			if first {
				logger.Info("waiting until " + goal)
			} else {
				logger.Info("still waiting until " + goal)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func WaitUntilCheck(ctx context.Context, sess *session.Session, logger *slog.Logger, check Check, delay time.Duration, interval time.Duration) error {
	if check.Func == nil {
		return nil
	}

	return WaitUntil(ctx, logger, check.Name, func(ctx context.Context) (bool, error) {
		return check.Func(ctx, sess)
	}, delay, interval)
}
