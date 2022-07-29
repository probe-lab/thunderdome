package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptrace"
	"os"
	"sync"
	"time"
)

type RequestTimes struct {
	Backend      string
	ConnectError bool
	Dropped      bool
	StatusCode   int
	ConnectTime  float64
	TTFB         float64
	TotalTime    float64
}

type Request struct {
	Method string            `json:"method"`
	URI    string            `json:"uri"`
	Body   []byte            `json:"body,omitempty"`
	Header map[string]string `json:"header"`
}

func main() {
	ctx := context.Background()

	scanner := bufio.NewScanner(os.Stdin)
	results := make(chan RequestTimes, 10000)

	backends := []string{"http://localhost:8080"}

	var wg sync.WaitGroup
	workers := make([]*Worker, len(backends))
	for i, backend := range backends {
		workers[i] = &Worker{
			Backend:  backend,
			Requests: make(chan *Request, 10),
		}

		wg.Add(1)
		go workers[i].Run(ctx, &wg, results)
	}

	go func() {
		for result := range results {
			fmt.Println(result)
		}
	}()

	for scanner.Scan() {
		json_bytes := scanner.Bytes()
		var req Request
		if err := json.Unmarshal(json_bytes, &req); err != nil {
			log.Fatalf("unmarshal: %v", err)
		}

		// TODO throttle to defined request rate
		for _, w := range workers {
			w.QueueRequest(ctx, &req, results)
		}

	}

	for _, w := range workers {
		close(w.Requests)
	}

	wg.Wait()

	close(results)
}

type Worker struct {
	Backend  string
	Requests chan *Request
}

func (w *Worker) QueueRequest(ctx context.Context, req *Request, results chan RequestTimes) {
	select {
	case w.Requests <- req:
	default:
		results <- RequestTimes{
			Backend: w.Backend,
			Dropped: true,
		}
	}
}

func (w *Worker) Run(ctx context.Context, wg *sync.WaitGroup, results chan RequestTimes) {
	defer wg.Done()
	for req := range w.Requests {
		result := w.timeGet(req)
		results <- result
	}
}

func (w *Worker) timeGet(r *Request) RequestTimes {
	req, _ := http.NewRequest("GET", w.Backend+r.URI, nil)
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
		return RequestTimes{
			Backend:      w.Backend,
			ConnectError: true,
		}
	}

	end = time.Now()
	totalTime = end.Sub(start)

	return RequestTimes{
		Backend:     w.Backend,
		StatusCode:  resp.StatusCode,
		ConnectTime: connectTime.Seconds(),
		TTFB:        ttfb.Seconds(),
		TotalTime:   totalTime.Seconds(),
	}
}
