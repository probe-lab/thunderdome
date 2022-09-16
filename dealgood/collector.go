package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spenczar/tdigest"
)

type RequestTiming struct {
	ExperimentName string
	TargetName     string
	ConnectError   bool
	TimeoutError   bool
	Dropped        bool
	StatusCode     int
	ConnectTime    time.Duration
	TTFB           time.Duration
	TotalTime      time.Duration
}

type Collector struct {
	timings        chan *RequestTiming
	sampleInterval time.Duration
	printer        SummaryPrinter

	ttfbHist            *prometheus.HistogramVec
	connectHist         *prometheus.HistogramVec
	totalHist           *prometheus.HistogramVec
	requestsCounter     *prometheus.CounterVec
	droppedCounter      *prometheus.CounterVec
	connectErrorCounter *prometheus.CounterVec
	timeoutErrorCounter *prometheus.CounterVec
	responsesCounter    *prometheus.CounterVec

	mu      sync.Mutex // guards access to samples
	samples map[string]MetricSample
}

func NewCollector(timings chan *RequestTiming, sampleInterval time.Duration, printer SummaryPrinter) (*Collector, error) {
	if sampleInterval <= 0 {
		sampleInterval = 1 * time.Second
	}

	coll := &Collector{
		timings:        timings,
		sampleInterval: sampleInterval,
		printer:        printer,
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
		"The total number of requests that were unable to connect to the target.",
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

	coll.timeoutErrorCounter, err = newCounterMetric(
		"timeout_error_total",
		"The total number of requests that timed out waiting for a response from the target.",
		[]string{"experiment", "target"},
	)
	if err != nil {
		return nil, fmt.Errorf("new counter: %w", err)
	}

	return coll, nil
}

func (c *Collector) Run(ctx context.Context) error {
	stats := make(map[string]*TargetStats)

	var sampleChan <-chan time.Time
	if c.sampleInterval > 0 && c.printer != nil {
		sampleTicker := time.NewTicker(c.sampleInterval)
		sampleChan = sampleTicker.C
		defer sampleTicker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-c.timings:
			if !ok {
				return nil
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
			} else if res.TimeoutError {
				st.TotalTimeoutErrors++
				c.timeoutErrorCounter.WithLabelValues(res.ExperimentName, res.TargetName).Add(1)
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

		case <-sampleChan:
			if c.printer == nil {
				continue
			}
			var summary Summary
			for target, v := range stats {
				st := *v
				ts := TargetSummary{
					Target: target,
					Measurements: MetricSample{
						TotalRequests:      st.TotalRequests,
						TotalConnectErrors: st.TotalConnectErrors,
						TotalTimeoutErrors: st.TotalTimeoutErrors,
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
					},
				}

				summary.Targets = append(summary.Targets, ts)
			}

			if err := c.printer(&summary); err != nil {
				log.Printf("error printing summary: %v", err)
			}

		}
	}
}

type TargetStats struct {
	TotalRequests      int
	TotalConnectErrors int
	TotalTimeoutErrors int
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

type SummaryPrinter func(*Summary) error

type Summary struct {
	Targets []TargetSummary `json:"targets"`
}

type TargetSummary struct {
	Target       string       `json:"target"`
	Measurements MetricSample `json:"measurements"`
}

type MetricSample struct {
	TotalRequests      int          `json:"total_requests"`
	TotalConnectErrors int          `json:"total_connect_errors"`
	TotalTimeoutErrors int          `json:"total_timeout_errors"`
	TotalDropped       int          `json:"total_dropped"`
	TotalHttp2XX       int          `json:"total_http_2xx"`
	TotalHttp3XX       int          `json:"total_http_3xx"`
	TotalHttp4XX       int          `json:"total_http_4xx"`
	TotalHttp5XX       int          `json:"total_http_5xx"`
	ConnectTime        MetricValues `json:"connect_time"`
	TTFB               MetricValues `json:"ttfb"`
	TotalTime          MetricValues `json:"total_time"`
}

// MetricValues contains timings in seconds
type MetricValues struct {
	Mean float64 `json:"mean"`
	Max  float64 `json:"max"`
	Min  float64 `json:"min"`
	P50  float64 `json:"p50"`
	P75  float64 `json:"p75"`
	P90  float64 `json:"p90"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
	P999 float64 `json:"p999"`
}

func secondsDesc(d int) string {
	if d == -1 {
		return "forever"
	}
	return durationDesc(time.Duration(d) * time.Second)
}

func durationDesc(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}
