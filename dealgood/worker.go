package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"os"
	"sync"
	"time"
)

type Worker struct {
	Backend        *Backend
	ExperimentName string
	Client         *http.Client
	PrintFailures  bool
}

func (w *Worker) Run(ctx context.Context, wg *sync.WaitGroup, results chan *RequestTiming) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-w.Backend.Requests:
			if !ok {
				return
			}
			result := w.timeRequest(req)

			// Check context again since it might have been canceled while we were
			// waiting for request
			select {
			case <-ctx.Done():
				return
			default:
			}

			results <- result
		}
	}
}

func (w *Worker) timeRequest(r *Request) *RequestTiming {
	req, _ := http.NewRequest("GET", w.Backend.BaseURL+r.URI, nil)
	for k, v := range r.Header {
		req.Header.Set(k, v)
	}

	if w.Backend.Host != "" {
		req.Host = w.Backend.Host
	}

	var start, end, connect time.Time
	var connectTime, ttfb, totalTime time.Duration
	trace := &httptrace.ClientTrace{
		// DNSStart: func(dsi httptrace.DNSStartInfo) { dns = time.Now() },
		// DNSDone: func(ddi httptrace.DNSDoneInfo) {
		// 	fmt.Printf("DNS Done: %v\n", time.Since(dns))
		// },

		// TLSHandshakeStart: func() { tlsHandshake = time.Now() },
		// TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
		// 	fmt.Printf("TLS Handshake: %v\n", time.Since(tlsHandshake))
		// },

		ConnectStart: func(network, addr string) {
			connect = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			connectTime = time.Since(connect)
		},

		GotFirstResponseByte: func() {
			ttfb = time.Since(start)
		},
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	start = time.Now()

	resp, err := w.Client.Do(req)
	if err != nil {
		if w.PrintFailures {
			fmt.Fprintf(os.Stderr, "%s %s => error %v\n", req.Method, req.URL, err)
		}
		return &RequestTiming{
			ExperimentName: w.ExperimentName,
			BackendName:    w.Backend.Name,
			ConnectError:   true,
		}
	}
	defer resp.Body.Close()
	io.Copy(ioutil.Discard, resp.Body)

	end = time.Now()
	totalTime = end.Sub(start)

	if w.PrintFailures {
		if resp.StatusCode/100 != 2 {
			fmt.Fprintf(os.Stderr, "%s %s => %s\n", req.Method, req.URL, resp.Status)
		}
	}

	return &RequestTiming{
		ExperimentName: w.ExperimentName,
		BackendName:    w.Backend.Name,
		StatusCode:     resp.StatusCode,
		ConnectTime:    connectTime,
		TTFB:           ttfb,
		TotalTime:      totalTime,
	}
}
