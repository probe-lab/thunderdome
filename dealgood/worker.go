package main

import (
	"context"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

type Worker struct {
	Backend  *Backend
	Requests chan *Request
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

	if host, ok := r.Header["Host"]; ok {
		req.Host = host
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &RequestTiming{
			BackendName:  w.Backend.Name,
			ConnectError: true,
		}
	}

	end = time.Now()
	totalTime = end.Sub(start)

	return &RequestTiming{
		BackendName: w.Backend.Name,
		StatusCode:  resp.StatusCode,
		ConnectTime: connectTime,
		TTFB:        ttfb,
		TotalTime:   totalTime,
	}
}
