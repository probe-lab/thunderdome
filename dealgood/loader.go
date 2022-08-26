package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

type Target struct {
	Name     string // short name of the target to be used in reports and metrics
	BaseURL  string // base URL of the target (without a path)
	Host     string
	Requests chan *Request // channel used to receive requests to be issued to the target
}

type Loader struct {
	Source         RequestSource // source of requests
	ExperimentName string
	Targets        []*Target           // targets to send load to
	Timings        chan *RequestTiming // channel to send timings to
	Rate           int                 // maximum number of requests per second per target
	Concurrency    int                 // number of workers per target
	Duration       int
	PrintFailures  bool
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
		// One unbuffered request channel per target, shared by all concurrent workers
		// for that target.
		if target.Requests == nil {
			target.Requests = make(chan *Request)
		}
		for j := 0; j < l.Concurrency; j++ {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					ServerName:         target.Host,
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
	lastRequestDone := time.Now()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case req := <-l.Source.Chan():

			timeSinceLast := time.Since(lastRequestDone)
			if timeSinceLast < requestInterval {
				time.Sleep(requestInterval - timeSinceLast)
			}

			select {
			case <-ctx.Done():
				break loop
			default:
			}

			for _, be := range l.Targets {
				select {
				case be.Requests <- &req:
					lastRequestDone = time.Now()
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
