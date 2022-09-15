package main

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// Some global counts that will be periodically logged
var (
	totalRequestsReceived atomic.Int64
	totalRequestsSent     atomic.Int64
)

type Health struct{}

func (h *Health) Run(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			log.Printf("sent %d requests of %d received", totalRequestsSent.Load(), totalRequestsReceived.Load())
		}
	}
}
