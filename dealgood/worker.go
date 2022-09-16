package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"strings"
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
			result := w.timeRequest(ctx, req)

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

func (w *Worker) timeRequest(ctx context.Context, r *Request) *RequestTiming {
	req, err := newRequest(ctx, w.Target, r)
	if err != nil {
		if w.PrintFailures {
			log.Printf("%s %s => error %v\n", r.Method, w.Target.BaseURL+r.URI, err)
		}
		return &RequestTiming{
			ExperimentName: w.ExperimentName,
			TargetName:     w.Target.Name,
			ConnectError:   true,
		}
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
			log.Printf("%s %s => error %v\n", req.Method, req.URL, err)
		}
		if os.IsTimeout(err) {
			return &RequestTiming{
				ExperimentName: w.ExperimentName,
				TargetName:     w.Target.Name,
				TimeoutError:   true,
			}
		}
		if err := resolveTarget(w.Target); err != nil {
			log.Printf("resolve %s => error %v\n", w.Target.RawHostPort, err)
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
			log.Printf("%s %s => %s\n", req.Method, req.URL, resp.Status)
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

func newRequest(ctx context.Context, t *Target, r *Request) (*http.Request, error) {
	req := &http.Request{
		Method: r.Method,
		URL: &url.URL{
			Scheme: t.URLScheme,
			Host:   t.HostPort(),
			Path:   r.URI,
		},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}

	if r.Body != nil {
		req.Body = io.NopCloser(bytes.NewReader(r.Body))
	}

	for k, v := range r.Header {
		req.Header.Set(k, v)
	}

	host := req.Header.Get("Host")
	// The live request log uses a hostname of backend to refer to the orginal host
	if host == "backend" || host == "" {
		host = t.HostName
		req.Header.Set("Host", t.HostName)
	}
	req.Host = host

	return req, nil
}

func targetsReady(ctx context.Context, targets []*Target, preProbeWait time.Duration, probeTimeout time.Duration) error {
	log.Printf("waiting for targets to be ready")
	if preProbeWait > 0 {
		log.Printf("waiting %s for targets to be started before probing", durationDesc(preProbeWait))
		time.Sleep(preProbeWait * time.Second)
	}

	var lastErr error

	start := time.Now()
	for {
		running := time.Since(start)
		if running > probeTimeout {
			return fmt.Errorf("unable to connect to all targets within %s: %w", durationDesc(probeTimeout), lastErr)
		}

		g, ctx := errgroup.WithContext(ctx)
		for _, target := range targets {
			target := target // avoid shadowing
			g.Go(func() error {
				if err := resolveTarget(target); err != nil {
					return err
				}

				tr := &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
						ServerName:         target.HostName,
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

				req, err := newRequest(ctx, target, &Request{Method: "GET", URI: "/"})
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
			log.Printf("all targets ready")
			return nil
		}

		log.Printf("ready check failed: %v", lastErr)
		time.Sleep(5 * time.Second)

	}
}

func resolve(name string) (string, error) {
	var host, port string
	var err error
	if strings.Contains(name, ":") {
		host, port, err = net.SplitHostPort(name)
		if err != nil {
			return name, fmt.Errorf("split host port: %w", err)
		}
	} else {
		host = name
		port = "8080" // assume gateway default
	}

	// Special case localhost
	if host == "localhost" {
		return name, nil
	}

	// name may already be using a raw IP address
	if net.ParseIP(host) != nil {
		return name, nil
	}

	if port != "" {
		// Lookup A record
		ips, err := net.LookupIP(host)
		if err != nil {
			var de *net.DNSError
			if errors.As(err, &de) {
				if de.Temporary() {
					return name, fmt.Errorf("temporary dns error: %w", de)
				}
				if de.Timeout() {
					return name, fmt.Errorf("dns timeout: %w", de)
				}
			}
		}

		// Pick first IP if we got one
		if len(ips) > 0 {
			return fmt.Sprintf("%s:%s", ips[0], port), nil
		}
	}

	// No A record so lookup SRV
	_, recs, err := net.DefaultResolver.LookupSRV(context.Background(), "", "", host)
	if err != nil {
		return name, fmt.Errorf("lookup srv: %w", err)
	}

	if len(recs) == 0 {
		return name, fmt.Errorf("no srv records found")
	}

	// Pick first record
	host = strings.TrimRight(recs[0].Target, ".")
	// Did we get an IP address
	if net.ParseIP(host) != nil {
		return fmt.Sprintf("%s:%d", host, recs[0].Port), nil
	}

	// attempt to resolve
	return resolve(fmt.Sprintf("%s:%d", host, recs[0].Port))
}

func resolveTarget(target *Target) error {
	hostport, err := resolve(target.RawHostPort)
	if err != nil {
		return fmt.Errorf("unable to resolve target %q: %w", target.RawHostPort, err)
	}
	if target.RawHostPort != hostport {
		target.SetHostPort(hostport)
		log.Printf("resolved %s to %s\n", target.RawHostPort, hostport)
	}

	return nil
}
