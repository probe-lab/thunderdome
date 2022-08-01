package main

import (
	"context"
	"sync"
	"time"
)

type Backend struct {
	Name     string
	BaseURL  string
	Requests chan *Request
}

type Loader struct {
	Source      RequestSource
	Rate        float64 // requests per second
	Concurrency int     // number of workers per backend
	Backends    []*Backend
}

func (l *Loader) Run(ctx context.Context, d time.Duration, results chan *RequestTiming) error {
	ctx, cancel := context.WithTimeout(ctx, d)
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
		go w.Run(ctx, &wg, results)
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
				results <- &RequestTiming{
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
