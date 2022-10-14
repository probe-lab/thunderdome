package main

import (
	"context"
	"time"
)

// WaitUntil repeatedly calls condition until condition returns true or an error or the context is cancelled.
// delay is the time to wait before calling condition for the first time.
// interval is the time to wait between each subsequent call to condition
func WaitUntil(ctx context.Context, condition func(context.Context) (bool, error), delay time.Duration, interval time.Duration) error {
	if delay > 0 {
		time.Sleep(delay)
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
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
