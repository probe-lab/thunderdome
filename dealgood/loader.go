package main

import (
	"context"
	"sync"
	"time"
)

type Backend struct {
	Name     string        // short name of the backend to be used in reports
	BaseURL  string        // base URL of the backend (without a path)
	Requests chan *Request // channel used to receive requests to be issued to the backend
}

type Loader struct {
	Source      RequestSource       // source of requests
	Backends    []*Backend          // backends to send load to
	Timings     chan *RequestTiming // channel to send timings to
	Rate        float64             // maximum number of requests per second per backend
	Concurrency int                 // number of workers per backend
	Duration    time.Duration
}

type LoadOptions struct {
	Rate        float64 // maximum number of requests per second per backend
	Concurrency int     // number of workers per backend
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
			be.Requests = make(chan *Request, 0)
		}
		for j := 0; j < l.Concurrency; j++ {
			workers = append(workers, &Worker{
				Backend: be,
			})
		}
	}

	var wg sync.WaitGroup
	wg.Add(len(workers))
	for _, w := range workers {
		go w.Run(ctx, &wg, l.Timings)
	}

	// requestInterval is the minimum time to wait between requests
	requestInterval := time.Duration(float64(time.Second) / l.Rate)
	lastRequestDone := time.Now()

	for l.Source.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		intervalToNextRequest := requestInterval - time.Since(lastRequestDone)
		if intervalToNextRequest > 0 {
			time.Sleep(intervalToNextRequest)
		}

		// TODO throttle to defined request rate
		for _, be := range l.Backends {
			select {
			case be.Requests <- l.Source.Request():
			default:
				l.Timings <- &RequestTiming{
					BackendName: be.Name,
					Dropped:     true,
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
