package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/spenczar/tdigest"
)

type RequestTiming struct {
	BackendName  string
	ConnectError bool
	Dropped      bool
	StatusCode   int
	ConnectTime  time.Duration
	TTFB         time.Duration
	TotalTime    time.Duration
}

type Collector struct {
	timings        chan *RequestTiming
	sampleInterval time.Duration

	mu      sync.Mutex // guards access to samples
	samples []map[string]BackendStats
}

func NewCollector(timings chan *RequestTiming, sampleInterval time.Duration) *Collector {
	if sampleInterval <= 0 {
		sampleInterval = 1 * time.Second
	}
	return &Collector{
		timings:        timings,
		sampleInterval: sampleInterval,
	}
}

func (c *Collector) Run(ctx context.Context) {
	stats := make(map[string]*BackendStats)

	sampleTicker := time.NewTicker(c.sampleInterval)
	defer sampleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case res, ok := <-c.timings:
			if !ok {
				return
			}

			st, ok := stats[res.BackendName]
			if !ok {
				st = &BackendStats{
					ConnectTime: tdigest.New(),
					TTFB:        tdigest.New(),
					TotalTime:   tdigest.New(),
				}
			}
			st.TotalRequests++
			if res.ConnectError {
				st.TotalConnectErrors++
			}
			if res.Dropped {
				st.TotalDropped++
			}
			if res.StatusCode/100 == 5 {
				st.TotalServerErrors++
			}
			st.ConnectTime.Add(res.ConnectTime.Seconds(), 1)
			st.TTFB.Add(res.TTFB.Seconds(), 1)
			st.TotalTime.Add(res.TotalTime.Seconds(), 1)
			stats[res.BackendName] = st

		case <-sampleTicker.C:
			sample := map[string]BackendStats{}
			for k, v := range stats {
				st := *v
				sample[k] = st
				fmt.Printf("requests: %d, dropped: %d, errored: %d, 5xx: %d, TTFB 50th: %.5f, TTFB 90th: %.5f, TTFB 99th: %.5f\n", st.TotalRequests, st.TotalDropped, st.TotalConnectErrors, st.TotalServerErrors, st.TTFB.Quantile(0.5), st.TTFB.Quantile(0.9), st.TTFB.Quantile(0.99))
			}
			c.mu.Lock()
			c.samples = append(c.samples, sample)
			c.mu.Unlock()

		}
	}
}

func (c *Collector) Latest() map[string]BackendStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.samples[len(c.samples)-1]
}

type BackendStats struct {
	TotalRequests      int
	TotalConnectErrors int
	TotalDropped       int
	TotalServerErrors  int
	ConnectTime        *tdigest.TDigest
	TTFB               *tdigest.TDigest
	TotalTime          *tdigest.TDigest
}
