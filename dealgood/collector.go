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
	samples []map[string]MetricSample
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
			switch res.StatusCode / 100 {
			case 2:
				st.TotalHttp2XX++
				st.TTFB.Add(res.TTFB.Seconds(), 1)
				st.TotalTime.Add(res.TotalTime.Seconds(), 1)
			case 3:
				st.TotalHttp3XX++
			case 4:
				st.TotalHttp4XX++
			case 5:
				st.TotalHttp5XX++
			}

			st.ConnectTime.Add(res.ConnectTime.Seconds(), 1)
			stats[res.BackendName] = st

		case <-sampleTicker.C:
			sample := map[string]MetricSample{}
			for k, v := range stats {
				st := *v
				sample[k] = MetricSample{
					TotalRequests:      st.TotalRequests,
					TotalConnectErrors: st.TotalConnectErrors,
					TotalDropped:       st.TotalDropped,
					TotalHttp2XX:       st.TotalHttp2XX,
					TotalHttp3XX:       st.TotalHttp3XX,
					TotalHttp4XX:       st.TotalHttp4XX,
					TotalHttp5XX:       st.TotalHttp5XX,
					ConnectTime: MetricVaues{
						Mean: st.ConnectTime.Quantile(0.5),
						Max:  st.ConnectTime.Quantile(1.0),
						Min:  st.ConnectTime.Quantile(0.0),
						P50:  st.ConnectTime.Quantile(0.50),
						P75:  st.ConnectTime.Quantile(0.75),
						P90:  st.ConnectTime.Quantile(0.90),
						P95:  st.ConnectTime.Quantile(0.95),
						P99:  st.ConnectTime.Quantile(0.99),
						P999: st.ConnectTime.Quantile(0.999),
					},
					TTFB: MetricVaues{
						Mean: st.TTFB.Quantile(0.5),
						Max:  st.TTFB.Quantile(1.0),
						Min:  st.TTFB.Quantile(0.0),
						P50:  st.TTFB.Quantile(0.50),
						P75:  st.TTFB.Quantile(0.75),
						P90:  st.TTFB.Quantile(0.90),
						P95:  st.TTFB.Quantile(0.95),
						P99:  st.TTFB.Quantile(0.99),
						P999: st.TTFB.Quantile(0.999),
					},
					TotalTime: MetricVaues{
						Mean: st.TotalTime.Quantile(0.5),
						Max:  st.TotalTime.Quantile(1.0),
						Min:  st.TotalTime.Quantile(0.0),
						P50:  st.TotalTime.Quantile(0.50),
						P75:  st.TotalTime.Quantile(0.75),
						P90:  st.TotalTime.Quantile(0.90),
						P95:  st.TotalTime.Quantile(0.95),
						P99:  st.TotalTime.Quantile(0.99),
						P999: st.TotalTime.Quantile(0.999),
					},
				}
				_ = fmt.Printf
				// fmt.Printf("requests: %d, dropped: %d, errored: %d, 5xx: %d, TTFB 50th: %.5f, TTFB 90th: %.5f, TTFB 99th: %.5f\n", st.TotalRequests, st.TotalDropped, st.TotalConnectErrors, st.TotalServerErrors, st.TTFB.Quantile(0.5), st.TTFB.Quantile(0.9), st.TTFB.Quantile(0.99))
			}
			c.mu.Lock()
			c.samples = append(c.samples, sample)
			c.mu.Unlock()

		}
	}
}

func (c *Collector) Latest() map[string]MetricSample {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.samples) == 0 {
		return map[string]MetricSample{}
	}
	return c.samples[len(c.samples)-1]
}

type BackendStats struct {
	TotalRequests      int
	TotalConnectErrors int
	TotalDropped       int
	TotalHttp2XX       int
	TotalHttp3XX       int
	TotalHttp4XX       int
	TotalHttp5XX       int
	ConnectTime        *tdigest.TDigest
	TTFB               *tdigest.TDigest
	TotalTime          *tdigest.TDigest
}

type MetricSample struct {
	TotalRequests      int
	TotalConnectErrors int
	TotalDropped       int
	TotalHttp2XX       int
	TotalHttp3XX       int
	TotalHttp4XX       int
	TotalHttp5XX       int
	ConnectTime        MetricVaues
	TTFB               MetricVaues
	TotalTime          MetricVaues
}

type MetricVaues struct {
	Mean float64
	Max  float64
	Min  float64
	P50  float64
	P75  float64
	P90  float64
	P95  float64
	P99  float64
	P999 float64
}
