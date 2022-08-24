package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/http2"
	"golang.org/x/sync/errgroup"
)

type Worker struct {
	Target         *Target
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
		case req, ok := <-w.Target.Requests:
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
	req, err := http.NewRequest(r.Method, w.Target.BaseURL+r.URI, nil)
	if err != nil {
		if w.PrintFailures {
			fmt.Fprintf(os.Stderr, "%s %s => error %v\n", r.Method, w.Target.BaseURL+r.URI, err)
		}
		return &RequestTiming{
			ExperimentName: w.ExperimentName,
			TargetName:     w.Target.Name,
			ConnectError:   true,
		}
	}

	for k, v := range r.Header {
		req.Header.Set(k, v)
	}

	if w.Target.Host != "" {
		req.Host = w.Target.Host
	}

	ctx, span := otel.Tracer("dealgood").Start(req.Context(), "HTTP "+req.Method, trace.WithAttributes(attribute.String("uri", r.URI)))
	defer span.End()

	prop := otel.GetTextMapPropagator()
	prop.Inject(ctx, propagation.HeaderCarrier(req.Header))
	req = req.WithContext(ctx)

	var start, end, connect time.Time
	var connectTime, ttfb, totalTime time.Duration
	trace := &httptrace.ClientTrace{
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
			TargetName:     w.Target.Name,
			ConnectError:   true,
		}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	end = time.Now()
	totalTime = end.Sub(start)

	if w.PrintFailures {
		if resp.StatusCode/100 != 2 {
			fmt.Fprintf(os.Stderr, "%s %s => %s\n", req.Method, req.URL, resp.Status)
		}
	}

	return &RequestTiming{
		ExperimentName: w.ExperimentName,
		TargetName:     w.Target.Name,
		StatusCode:     resp.StatusCode,
		ConnectTime:    connectTime,
		TTFB:           ttfb,
		TotalTime:      totalTime,
	}
}

func targetsReady(ctx context.Context, targets []*TargetJSON, quiet bool) error {
	const readyTimeout = 60
	var lastErr error
	start := time.Now()
	for {
		running := time.Since(start)
		if running > readyTimeout*time.Second {
			return fmt.Errorf("unable to connect to all targets within %s", durationDesc(readyTimeout))
		}

		g, ctx := errgroup.WithContext(ctx)
		for _, target := range targets {
			target = target // avoid shadowing
			g.Go(func() error {
				tr := &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
						ServerName:         target.Host,
					},
					MaxIdleConnsPerHost: http.DefaultMaxIdleConnsPerHost,
					DisableCompression:  true,
					DisableKeepAlives:   true,
				}
				http2.ConfigureTransport(tr)

				hc := &http.Client{
					Transport: tr,
					Timeout:   2 * time.Second,
				}
				req, err := http.NewRequest("GET", target.BaseURL, nil)
				if err != nil {
					return fmt.Errorf("new request to target: %w", err)
				}
				req = req.WithContext(ctx)

				resp, err := hc.Do(req)
				if err != nil {
					return fmt.Errorf("request: %w", err)
				}
				defer resp.Body.Close()
				io.Copy(io.Discard, resp.Body)
				return nil
			})

		}

		lastErr = g.Wait()
		if lastErr == nil {
			if !quiet {
				fmt.Printf("all targets ready\n")
			}
			// All requests succeeded
			return nil
		}

		if !quiet {
			fmt.Printf("ready check failed: %v\n", lastErr)
		}
		time.Sleep(5 * time.Second)

	}

	return lastErr
}
