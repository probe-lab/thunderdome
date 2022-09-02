package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spenczar/tdigest"
)

type RequestTiming struct {
	ExperimentName string
	TargetName     string
	ConnectError   bool
	Dropped        bool
	StatusCode     int
	ConnectTime    time.Duration
	TTFB           time.Duration
	TotalTime      time.Duration
}

type Collector struct {
	timings             chan *RequestTiming
	sampleInterval      time.Duration
	ttfbHist            *prometheus.HistogramVec
	connectHist         *prometheus.HistogramVec
	totalHist           *prometheus.HistogramVec
	requestsCounter     *prometheus.CounterVec
	droppedCounter      *prometheus.CounterVec
	connectErrorCounter *prometheus.CounterVec
	responsesCounter    *prometheus.CounterVec

	mu      sync.Mutex // guards access to samples
	samples map[string]MetricSample
}

func NewCollector(timings chan *RequestTiming, sampleInterval time.Duration) (*Collector, error) {
	if sampleInterval <= 0 {
		sampleInterval = 1 * time.Second
	}

	coll := &Collector{
		timings:        timings,
		sampleInterval: sampleInterval,
	}

	var err error
	coll.ttfbHist, err = newHistogramMetric(
		"ttfb_seconds",
		"The time till the first byte is received for successful gateway requests.",
		[]string{"experiment", "target"},
	)
	if err != nil {
		return nil, fmt.Errorf("new histogram: %w", err)
	}
	coll.connectHist, err = newHistogramMetric(
		"connect_time_seconds",
		"The time to connect to the target gateway.",
		[]string{"experiment", "target"},
	)
	if err != nil {
		return nil, fmt.Errorf("new histogram: %w", err)
	}
	coll.totalHist, err = newHistogramMetric(
		"request_time_seconds",
		"The total time taken for successful gateway requests.",
		[]string{"experiment", "target"},
	)
	if err != nil {
		return nil, fmt.Errorf("new histogram: %w", err)
	}

	coll.requestsCounter, err = newCounterMetric(
		"requests_total",
		"The total number of requests attempted.",
		[]string{"experiment", "target"},
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	coll.droppedCounter, err = newCounterMetric(
		"dropped_total",
		"The total number of requests that were dropped because there were too many requests already in-flight.",
		[]string{"experiment", "target"},
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	coll.connectErrorCounter, err = newCounterMetric(
		"connect_error_total",
		"The total number of requests that unable to connect to the target.",
		[]string{"experiment", "target"},
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	coll.responsesCounter, err = newCounterMetric(
		"responses_total",
		"The total number of responses received.",
		[]string{"experiment", "target", "code"},
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	return coll, nil
}

func (c *Collector) Run(ctx context.Context) {
	stats := make(map[string]*TargetStats)

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

			st, ok := stats[res.TargetName]
			if !ok {
				st = &TargetStats{
					ConnectTime: NewTimeMetric(),
					TTFB:        NewTimeMetric(),
					TotalTime:   NewTimeMetric(),
				}
			}
			st.TotalRequests++
			c.requestsCounter.WithLabelValues(res.ExperimentName, res.TargetName).Add(1)
			if res.ConnectError {
				st.TotalConnectErrors++
				c.connectErrorCounter.WithLabelValues(res.ExperimentName, res.TargetName).Add(1)
			} else if res.Dropped {
				st.TotalDropped++
				c.droppedCounter.WithLabelValues(res.ExperimentName, res.TargetName).Add(1)
			} else {
				st.ConnectTime.Add(res.ConnectTime.Seconds())
				c.connectHist.WithLabelValues(res.ExperimentName, res.TargetName).Observe(res.ConnectTime.Seconds())
				c.responsesCounter.WithLabelValues(res.ExperimentName, res.TargetName, strconv.Itoa(res.StatusCode)).Add(1)

				switch res.StatusCode / 100 {
				case 2:
					st.TotalHttp2XX++
					st.TTFB.Add(res.TTFB.Seconds())
					st.TotalTime.Add(res.TotalTime.Seconds())
					c.ttfbHist.WithLabelValues(res.ExperimentName, res.TargetName).Observe(res.TTFB.Seconds())
					c.totalHist.WithLabelValues(res.ExperimentName, res.TargetName).Observe(res.TotalTime.Seconds())
				case 3:
					st.TotalHttp3XX++
				case 4:
					st.TotalHttp4XX++
				case 5:
					st.TotalHttp5XX++
				}
			}

			stats[res.TargetName] = st

		case <-sampleTicker.C:
			samples := map[string]MetricSample{}
			for k, v := range stats {
				st := *v
				samples[k] = MetricSample{
					TotalRequests:      st.TotalRequests,
					TotalConnectErrors: st.TotalConnectErrors,
					TotalDropped:       st.TotalDropped,
					TotalHttp2XX:       st.TotalHttp2XX,
					TotalHttp3XX:       st.TotalHttp3XX,
					TotalHttp4XX:       st.TotalHttp4XX,
					TotalHttp5XX:       st.TotalHttp5XX,
					ConnectTime: MetricValues{
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
					TTFB: MetricValues{
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
					TotalTime: MetricValues{
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
			c.samples = samples
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
	samples := map[string]MetricSample{}
	for k, v := range c.samples {
		samples[k] = v
	}
	return samples
}

type TargetStats struct {
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
	ConnectTime        MetricValues
	TTFB               MetricValues
	TotalTime          MetricValues
}

// MetricValues contains timings in seconds
type MetricValues struct {
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

func newHistogramMetric(name string, help string, labels []string) (*prometheus.HistogramVec, error) {
	m := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "thunderdome",
			Subsystem: "dealgood",
			Name:      name,
			Help:      help,
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 240},
		},
		labels,
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			return nil, fmt.Errorf("register %s histogram: %w", name, err)
		}
	}
	return m, nil
}

func newCounterMetric(name string, help string, labels []string) (*prometheus.CounterVec, error) {
	m := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "thunderdome",
			Subsystem: "dealgood",
			Name:      name,
			Help:      help,
		},
		labels,
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(*prometheus.CounterVec)
		} else {
			return nil, fmt.Errorf("register %s counter: %w", name, err)
		}
	}
	return m, nil
}

func newGaugeMetric(name string, help string, labels []string) (*prometheus.GaugeVec, error) {
	m := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "thunderdome",
			Subsystem: "dealgood",
			Name:      name,
			Help:      help,
		},
		labels,
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			return nil, fmt.Errorf("register %s gauge: %w", name, err)
		}
	}
	return m, nil
}
