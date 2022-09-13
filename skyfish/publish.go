package main

import (
	"context"
	"fmt"
)

type Publisher struct {
	reqs <-chan Request
}

func NewPublisher(reqs <-chan Request) (*Publisher, error) {
	return &Publisher{
		reqs: reqs,
	}, nil
}

// Run starts running the publisher and blocks until the context is canceled or a fatal
// error is encountered.
func (p *Publisher) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r, ok := <-p.reqs:
			if !ok {
				return fmt.Errorf("request channel closed")
			}
			fmt.Printf("request: %s\n", r.URI)

		}
	}
}

// Shutdown gracefully shuts down the publisher without interrupting any active
// connections. If the context is canceled the function should return the context error.
func (p *Publisher) Shutdown(ctx context.Context) error {
	return nil
}
