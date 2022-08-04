package main

import (
	"context"
	"fmt"
	"math"
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
					ConnectTime: NewTimeMetric(),
					TTFB:        NewTimeMetric(),
					TotalTime:   NewTimeMetric(),
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
				st.TTFB.Add(res.TTFB.Seconds())
				st.TotalTime.Add(res.TotalTime.Seconds())
			case 3:
				st.TotalHttp3XX++
			case 4:
				st.TotalHttp4XX++
			case 5:
				st.TotalHttp5XX++
			}

			st.ConnectTime.Add(res.ConnectTime.Seconds())
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
						Mean: st.ConnectTime.Mean(),
						Max:  st.ConnectTime.Max,
						Min:  st.ConnectTime.Min,
						P50:  st.ConnectTime.Digest.Quantile(0.50),
						P75:  st.ConnectTime.Digest.Quantile(0.75),
						P90:  st.ConnectTime.Digest.Quantile(0.90),
						P95:  st.ConnectTime.Digest.Quantile(0.95),
						P99:  st.ConnectTime.Digest.Quantile(0.99),
						P999: st.ConnectTime.Digest.Quantile(0.999),
					},
					TTFB: MetricVaues{
						Mean: st.TTFB.Mean(),
						Max:  st.TTFB.Max,
						Min:  st.TTFB.Min,
						P50:  st.TTFB.Digest.Quantile(0.50),
						P75:  st.TTFB.Digest.Quantile(0.75),
						P90:  st.TTFB.Digest.Quantile(0.90),
						P95:  st.TTFB.Digest.Quantile(0.95),
						P99:  st.TTFB.Digest.Quantile(0.99),
						P999: st.TTFB.Digest.Quantile(0.999),
					},
					TotalTime: MetricVaues{
						Mean: st.TotalTime.Mean(),
						Max:  st.TotalTime.Max,
						Min:  st.TotalTime.Min,
						P50:  st.TotalTime.Digest.Quantile(0.50),
						P75:  st.TotalTime.Digest.Quantile(0.75),
						P90:  st.TotalTime.Digest.Quantile(0.90),
						P95:  st.TotalTime.Digest.Quantile(0.95),
						P99:  st.TotalTime.Digest.Quantile(0.99),
						P999: st.TotalTime.Digest.Quantile(0.999),
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
	ConnectTime        *TimeMetric
	TTFB               *TimeMetric
	TotalTime          *TimeMetric
}

type TimeMetric struct {
	Digest *tdigest.TDigest
	Count  int
	Sum    float64
	Min    float64
	Max    float64
}

func NewTimeMetric() *TimeMetric {
	return &TimeMetric{
		Digest: tdigest.New(),
		Min:    math.NaN(),
		Max:    math.NaN(),
	}
}

func (t *TimeMetric) Add(v float64) {
	t.Count++
	t.Sum += v
	t.Digest.Add(v, 1)
	if math.IsNaN(t.Min) || v < t.Min {
		t.Min = v
	}
	if math.IsNaN(t.Max) || v > t.Max {
		t.Max = v
	}
}

func (t *TimeMetric) Mean() float64 {
	if t.Count == 0 {
		return 0
	}
	return t.Sum / float64(t.Count)
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
