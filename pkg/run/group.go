package run

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"
)

type Runnable interface {
	// Run starts running the component and blocks until the context is canceled, Shutdown is // called or a fatal error is encountered.
	Run(context.Context) error
}

type Group struct {
	runnables []Runnable
}

func (a *Group) Add(r Runnable) {
	a.runnables = append(a.runnables, r)
}

func (a *Group) RunAndWait(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)

	for i := range a.runnables {
		r := a.runnables[i]
		g.Go(func() error { return r.Run(ctx) })
	}

	// Ensure components stop if we receive a terminating operating system signal.
	g.Go(func() error {
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, syscall.SIGTERM, syscall.SIGINT)
		select {
		case <-interrupt:
			cancel()
		case <-ctx.Done():
		}
		return nil
	})

	// Wait for all servers to run to completion.
	if err := g.Wait(); err != nil {
		if !errors.Is(err, context.Canceled) {
			return err
		}
	}
	return nil
}
