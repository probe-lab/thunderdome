package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"
)

func nogui(ctx context.Context, source RequestSource, exp *ExperimentJSON) error {
	timings := make(chan *RequestTiming, 10000)
	defer func() {
		close(timings)
	}()

	coll := NewCollector(timings, 100*time.Millisecond)
	go coll.Run(ctx)

	go printTimings(ctx, coll, exp)

	l := &Loader{
		Source:      source,
		Rate:        exp.Rate,
		Concurrency: exp.Concurrency,
		Duration:    time.Duration(exp.Duration) * time.Second,
		Timings:     timings,
	}

	for _, be := range exp.Backends {
		l.Backends = append(l.Backends, &Backend{
			Name:    be.Name,
			BaseURL: be.BaseURL,
		})
	}

	if err := l.Send(ctx); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "loader stopped: %v", err)
		}
	}
	return nil
}

func printTimings(ctx context.Context, coll *Collector, exp *ExperimentJSON) {
	start := time.Now()
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	fmt.Printf("Time: %s\n", start.Format(time.RFC1123Z))
	fmt.Printf("Experiment: %s\n", exp.Name)
	fmt.Printf("Duration: %ds\n", exp.Duration)
	fmt.Printf("Request rate: %d\n", exp.Rate)
	fmt.Printf("Request concurrency: %d\n", exp.Concurrency)
	fmt.Println("Backends:")
	for _, be := range exp.Backends {
		fmt.Printf("  %s (%s)\n", be.Name, be.BaseURL)
	}
	fmt.Println("")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	fmt.Fprintln(w, "time\tbackend\trequests\tconn errs\tdropped\t5xx errs\tTTFB P50\tTTFB P90\tTTFB P90")
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:

			latest := coll.Latest()

			for _, be := range exp.Backends {
				st, ok := latest[be.Name]
				if !ok {
					continue
				}

				_ = st
				fmt.Fprintf(w, "% 5d\t%12s\t% 9d\t% 9d\t% 9d\t% 9d\t%8.4f\t%8.4f\t%8.4f\n", now.Sub(start)/time.Second, be.Name, st.TotalRequests, st.TotalConnectErrors, st.TotalDropped, st.TotalHttp5XX, st.TTFB.P50, st.TTFB.P90, st.TTFB.P99)

			}
			w.Flush()
		}
	}
}
