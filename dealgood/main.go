package main

import (
	"context"
	"log"
	"time"
)

func main() {
	ctx := context.Background()

	timings := make(chan *RequestTiming, 10000)

	coll := NewCollector(timings)
	go coll.Run(ctx)

	l := &Loader{
		// Source: NewStdinRequestSource(),
		Source:      NewRandomRequestSource(sampleRequests),
		Rate:        1000, // per second
		Concurrency: 50,
		Backends: []*Backend{
			{
				Name:    "local",
				BaseURL: "http://localhost:8080",
			},
		},
	}

	if err := l.Run(ctx, 60*time.Second, timings); err != nil {
		log.Printf("loader error: %v", err)
	}

	close(timings)
}
