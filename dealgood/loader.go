package main

import (
	"context"
	"crypto/tls"
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
	for _, be := range l.Targets {
		// One unbuffered request channel per target, shared by all concurrent workers
		// for that target.
		if be.Requests == nil {
			be.Requests = make(chan *Request)
		}
		for j := 0; j < l.Concurrency; j++ {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					ServerName:         be.Host,
				},
				MaxIdleConnsPerHost: http.DefaultMaxIdleConnsPerHost,
				DisableCompression:  true,
				DisableKeepAlives:   true,
			}
			http2.ConfigureTransport(tr)

			workers = append(workers, &Worker{
				Target:         be,
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

	// requestInterval is the minimum time to wait between requests
	requestInterval := time.Duration(float64(time.Second) / float64(l.Rate))
	lastRequestDone := time.Now()

	for l.Source.Next() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		intervalToNextRequest := requestInterval - time.Since(lastRequestDone)
		if intervalToNextRequest > 0 {
			time.Sleep(intervalToNextRequest)
		}

		req := l.Source.Request()
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
		lastRequestDone = time.Now()
	}
	for _, be := range l.Targets {
		close(be.Requests)
	}
	wg.Wait()

	return l.Source.Err()
}
