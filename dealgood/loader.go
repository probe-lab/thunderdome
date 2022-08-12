package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

type Backend struct {
	Name     string // short name of the backend to be used in reports
	BaseURL  string // base URL of the backend (without a path)
	Host     string
	Requests chan *Request // channel used to receive requests to be issued to the backend
}

type Loader struct {
	Source         RequestSource // source of requests
	ExperimentName string
	Backends       []*Backend          // backends to send load to
	Timings        chan *RequestTiming // channel to send timings to
	Rate           int                 // maximum number of requests per second per backend
	Concurrency    int                 // number of workers per backend
	Duration       time.Duration
	PrintFailures  bool
}

type LoadOptions struct {
	Rate        int // maximum number of requests per second per backend
	Concurrency int // number of workers per backend
	Duration    time.Duration
}

// Send sends requests to each backend until the duration has passed or the context is canceled.
func (l *Loader) Send(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, l.Duration)
	defer cancel()

	workers := make([]*Worker, 0, len(l.Backends)*l.Concurrency)
	for _, be := range l.Backends {
		// One unbuffered request channel per backend, shared by all concurrent workers
		// for that backend.
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
				Backend:        be,
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
		for _, be := range l.Backends {
			select {
			case be.Requests <- &req:
			default:
				l.Timings <- &RequestTiming{
					ExperimentName: l.ExperimentName,
					BackendName:    be.Name,
					Dropped:        true,
				}
			}
		}
		lastRequestDone = time.Now()
	}
	for _, be := range l.Backends {
		close(be.Requests)
	}
	wg.Wait()

	return l.Source.Err()
}
