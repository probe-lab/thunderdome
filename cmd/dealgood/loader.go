package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/plprobelab/thunderdome/pkg/request"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/http2"
)

type Loader struct {
	Source         RequestSource // source of requests
	ExperimentName string
	Targets        []*Target           // targets to send load to
	Timings        chan *RequestTiming // channel to send timings to
	Rate           int                 // maximum number of requests per second per target
	Concurrency    int                 // number of workers per target
	Duration       int
	PrintFailures  bool

	streamLagGauge        *prometheus.GaugeVec
	streamIntervalGauge   *prometheus.GaugeVec
	streamRequestsCounter *prometheus.CounterVec
	streamWaitCounter     *prometheus.CounterVec
	targetsGauge          *prometheus.GaugeVec
	rateGauge             *prometheus.GaugeVec
	concurrencyGauge      *prometheus.GaugeVec
}

func NewLoader(experimentName string, targets []*Target, source RequestSource, timings chan *RequestTiming, maxRate int, maxConcurrency int, duration int) (*Loader, error) {
	l := &Loader{
		Source:         source,
		ExperimentName: experimentName,
		Targets:        targets,
		Rate:           maxRate,
		Concurrency:    maxConcurrency,
		Duration:       duration,
		Timings:        timings,
	}

	var err error
	l.streamLagGauge, err = newGaugeMetric(
		"stream_lag_seconds",
		"The number of seconds between a request being placed in the stream before being sent to targets. Increasing values indicate the targets are falling behind the stream.",
		[]string{"experiment"},
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	l.streamIntervalGauge, err = newGaugeMetric(
		"stream_interval_seconds",
		"The number of seconds between a request being read from the incoming stream and being send to targets. Higher values indicate the stream is falling behind the targets, leading to starvation.",
		[]string{"experiment"},
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	l.streamWaitCounter, err = newCounterMetric(
		"stream_wait_total",
		"The number of times the loader had to wait for an incoming request from the stream. This indicates starvation.",
		[]string{"experiment"},
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	l.streamRequestsCounter, err = newCounterMetric(
		"stream_requests_total",
		"The number of requests read from the stream.",
		[]string{"experiment"},
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	l.targetsGauge, err = newGaugeMetric(
		"experiment_targets",
		"The number of targets specified for the experiment.",
		[]string{"experiment"},
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	l.rateGauge, err = newGaugeMetric(
		"experiment_request_rate",
		"The maximum request rate specified for the experiment.",
		[]string{"experiment"},
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	l.concurrencyGauge, err = newGaugeMetric(
		"experiment_concurrency",
		"The maximum number of concurrent requests specified for the experiment.",
		[]string{"experiment"},
	)
	if err != nil {
		return nil, fmt.Errorf("new gauge: %w", err)
	}

	return l, nil
}

// Send sends requests to each target until the duration has passed or the context is canceled.
func (l *Loader) Send(ctx context.Context) error {
	var cancel func()
	if l.Duration > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(l.Duration)*time.Second)
		defer cancel()
	}

	workers := make([]*Worker, 0, len(l.Targets)*l.Concurrency)
	for _, target := range l.Targets {
		for j := 0; j < l.Concurrency; j++ {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					ServerName:         target.HostName,
				},
				MaxIdleConnsPerHost: http.DefaultMaxIdleConnsPerHost,
				DisableCompression:  true,
				DisableKeepAlives:   true,
			}
			http2.ConfigureTransport(tr)

			workers = append(workers, &Worker{
				Target:         target,
				ExperimentName: l.ExperimentName,
				Client: &http.Client{
					Transport: tr,
					Timeout:   30 * time.Second,
				},
				PrintFailures: l.PrintFailures,
			})
		}
	}

	var wg sync.WaitGroup
	wg.Add(len(workers))
	for _, w := range workers {
		go w.Run(ctx, &wg, l.Timings)
	}

	if err := l.Source.Start(); err != nil {
		return fmt.Errorf("start source: %w", err)
	}

	requestInterval := time.Duration(float64(time.Second) / float64(l.Rate))

	tick := time.NewTicker(requestInterval)
	defer tick.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-tick.C:
			l.targetsGauge.WithLabelValues(l.ExperimentName).Set(float64(len(l.Targets)))
			l.rateGauge.WithLabelValues(l.ExperimentName).Set(float64(l.Rate))
			l.concurrencyGauge.WithLabelValues(l.ExperimentName).Set(float64(l.Concurrency))

			var req request.Request
			var ok bool

			// Do we have a request available
			select {
			case <-ctx.Done():
				break loop
			case req, ok = <-l.Source.Chan():
			default:
				// No request ready so report that
				l.streamWaitCounter.WithLabelValues(l.ExperimentName).Add(1)

				// Now wait for the request
				select {
				case <-ctx.Done():
					break loop
				case req, ok = <-l.Source.Chan():
				}
			}
			if !ok {
				// Channel was closed so source is terminated
				break loop
			}
			// Report that we got a request
			l.streamRequestsCounter.WithLabelValues(l.ExperimentName).Add(1)

			// report how far behind the stream we are
			l.streamLagGauge.WithLabelValues(l.ExperimentName).Set(time.Since(req.Timestamp).Seconds())

			for _, be := range l.Targets {
				select {
				case be.Requests <- &req:
				default:
					l.Timings <- &RequestTiming{
						ExperimentName: l.ExperimentName,
						TargetName:     be.Name,
						Dropped:        true,
					}
				}
			}
		}
	}

	for _, be := range l.Targets {
		close(be.Requests)
	}
	wg.Wait()

	if err := l.Source.Err(); err != nil {
		return fmt.Errorf("source: %w", err)
	}

	return nil
}
