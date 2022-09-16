package main

import (
	"fmt"
	"log"
)

func NullSummaryPrinter(s *Summary) error {
	return nil
}

func DumpFullSummary(s *Summary) error {
	for i, ts := range s.Targets {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("Target:  %s\n", ts.Target)
		fmt.Printf("------------------------------\n")

		st := ts.Measurements
		connectedRequests := st.TotalRequests - st.TotalConnectErrors - st.TotalDropped

		fmt.Printf("Issued:          %9d\n", st.TotalRequests)
		fmt.Printf("Connect Errors:  %9d (%6.2f%%)\n", st.TotalConnectErrors, 100*float64(st.TotalConnectErrors)/float64(st.TotalRequests))
		fmt.Printf("Timeout Errors:  %9d (%6.2f%%)\n", st.TotalTimeoutErrors, 100*float64(st.TotalTimeoutErrors)/float64(st.TotalRequests))
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

	return nil
}

func DumpBriefSummary(s *Summary) error {
	return printBriefSummary(s, func(line string) {
		fmt.Println(line)
	})
}

func LogBriefSummary(s *Summary) error {
	return printBriefSummary(s, func(line string) {
		log.Print(line)
	})
}

func printBriefSummary(s *Summary, p func(string)) error {
	for _, ts := range s.Targets {
		st := ts.Measurements
		connectedRequests := st.TotalRequests - st.TotalConnectErrors - st.TotalDropped

		line := fmt.Sprintf("target: %s; TTFB P99: %.3fms; reqs: %d; dropped: %.2f%%; timeout: %.2f%%; server errors: %.2f%%",
			ts.Target,
			st.TTFB.P99*1000,
			st.TotalRequests,
			100*float64(st.TotalDropped)/float64(st.TotalRequests),
			100*float64(st.TotalTimeoutErrors)/float64(st.TotalRequests),
			100*float64(st.TotalHttp5XX)/float64(connectedRequests),
		)
		p(line)
	}

	return nil
}
