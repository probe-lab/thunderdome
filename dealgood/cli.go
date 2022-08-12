package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"
)

func nogui(ctx context.Context, source RequestSource, exp *ExperimentJSON, printHeader bool, printTimings bool, printFailures bool) error {
	timings := make(chan *RequestTiming, 10000)
	defer func() {
		close(timings)
	}()

	coll, err := NewCollector(timings, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("new collector: %w", err)
	}
	go coll.Run(ctx)

	if printHeader {
		fmt.Printf("Time: %s\n", time.Now().Format(time.RFC1123Z))
		fmt.Printf("Experiment: %s\n", exp.Name)
		fmt.Printf("Duration: %ds\n", exp.Duration)
		fmt.Printf("Request rate: %d\n", exp.Rate)
		fmt.Printf("Request concurrency: %d\n", exp.Concurrency)
		fmt.Println("Backends:")
		for _, be := range exp.Backends {
			fmt.Printf("  %s (%s)\n", be.Name, be.BaseURL)
		}
		fmt.Println("")
	}

	if printTimings {
		go printCollectedTimings(ctx, coll, exp)
	}

	l := &Loader{
		Source:         source,
		ExperimentName: exp.Name,
		Rate:           exp.Rate,
		Concurrency:    exp.Concurrency,
		Duration:       time.Duration(exp.Duration) * time.Second,
		Timings:        timings,
		PrintFailures:  printFailures,
	}

	for _, be := range exp.Backends {
		l.Backends = append(l.Backends, &Backend{
			Name:    be.Name,
			BaseURL: be.BaseURL,
			Host:    be.Host,
		})
	}

	if err := l.Send(ctx); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "loader stopped: %v", err)
		}
	}

	latest := coll.Latest()
	printSampleTimings(ctx, latest, exp)

	return nil
}

func printCollectedTimings(ctx context.Context, coll *Collector, exp *ExperimentJSON) {
	start := time.Now()
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

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
				fmt.Fprintf(w, "% 5d\t%12s\t% 9d\t% 9d\t% 9d\t% 9d\t%9.3f\t%9.3f\t%9.3f\n", now.Sub(start)/time.Second, be.Name, st.TotalRequests, st.TotalConnectErrors, st.TotalDropped, st.TotalHttp5XX, st.TTFB.P50*1000, st.TTFB.P90*1000, st.TTFB.P99*1000)

			}
			w.Flush()
		}
	}
}

func printSampleTimings(ctx context.Context, sample map[string]MetricSample, exp *ExperimentJSON) {
	for i, be := range exp.Backends {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("Backend:  %s\n", be.Name)
		fmt.Printf("Base URL: %s\n", be.BaseURL)
		fmt.Printf("------------------------------\n")

		st, ok := sample[be.Name]
		if !ok {
			fmt.Println("no metrics available")
			continue
		}

		connectedRequests := st.TotalRequests - st.TotalConnectErrors - st.TotalDropped

		fmt.Printf("Issued:          %9d\n", st.TotalRequests)
		fmt.Printf("Connect Errors:  %9d (%6.2f%%)\n", st.TotalConnectErrors, 100*float64(st.TotalConnectErrors)/float64(st.TotalRequests))
		fmt.Printf("Dropped:         %9d (%6.2f%%)\n", st.TotalDropped, 100*float64(st.TotalDropped)/float64(st.TotalRequests))
		fmt.Printf("Connected:       %9d (%6.2f%%)\n", connectedRequests, 100*float64(connectedRequests)/float64(st.TotalRequests))
		fmt.Println()
		fmt.Printf("HTTP 2XX Responses: %9d (%6.2f%%)\n", st.TotalHttp2XX, 100*float64(st.TotalHttp2XX)/float64(connectedRequests))
		fmt.Printf("HTTP 3XX Responses: %9d (%6.2f%%)\n", st.TotalHttp3XX, 100*float64(st.TotalHttp3XX)/float64(connectedRequests))
		fmt.Printf("HTTP 4XX Responses: %9d (%6.2f%%)\n", st.TotalHttp4XX, 100*float64(st.TotalHttp4XX)/float64(connectedRequests))
		fmt.Printf("HTTP 5XX Responses: %9d (%6.2f%%)\n", st.TotalHttp5XX, 100*float64(st.TotalHttp5XX)/float64(connectedRequests))
		fmt.Println()
		fmt.Printf("Time to connect\n")
		fmt.Printf("  Mean: %9.3fms\n", st.ConnectTime.Mean*1000)
		fmt.Printf("  Min:  %9.3fms\n", st.ConnectTime.Min*1000)
		fmt.Printf("  Max:  %9.3fms\n", st.ConnectTime.Max*1000)
		fmt.Printf("  P50:  %9.3fms\n", st.ConnectTime.P50*1000)
		fmt.Printf("  P90:  %9.3fms\n", st.ConnectTime.P90*1000)
		fmt.Printf("  P95:  %9.3fms\n", st.ConnectTime.P95*1000)
		fmt.Printf("  P99:  %9.3fms\n", st.ConnectTime.P99*1000)
		fmt.Println()
		fmt.Printf("Time to first byte\n")
		fmt.Printf("  Mean: %9.3fms\n", st.TTFB.Mean*1000)
		fmt.Printf("  Min:  %9.3fms\n", st.TTFB.Min*1000)
		fmt.Printf("  Max:  %9.3fms\n", st.TTFB.Max*1000)
		fmt.Printf("  P50:  %9.3fms\n", st.TTFB.P50*1000)
		fmt.Printf("  P90:  %9.3fms\n", st.TTFB.P90*1000)
		fmt.Printf("  P95:  %9.3fms\n", st.TTFB.P95*1000)
		fmt.Printf("  P99:  %9.3fms\n", st.TTFB.P99*1000)
		fmt.Println()
		fmt.Printf("Total request time\n")
		fmt.Printf("  Mean: %9.3fms\n", st.TotalTime.Mean*1000)
		fmt.Printf("  Min:  %9.3fms\n", st.TotalTime.Min*1000)
		fmt.Printf("  Max:  %9.3fms\n", st.TotalTime.Max*1000)
		fmt.Printf("  P50:  %9.3fms\n", st.TotalTime.P50*1000)
		fmt.Printf("  P90:  %9.3fms\n", st.TotalTime.P90*1000)
		fmt.Printf("  P95:  %9.3fms\n", st.TotalTime.P95*1000)
		fmt.Printf("  P99:  %9.3fms\n", st.TotalTime.P99*1000)
	}
}
