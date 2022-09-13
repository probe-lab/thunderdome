package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	promexp "contrib.go.opencensus.io/exporter/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	prom "github.com/prometheus/client_golang/prometheus"
	"go.opencensus.io/stats/view"
)

type PrometheusServer struct {
	addr string
	pe   *promexp.Exporter
}

func NewPrometheusServer(addr string) (*PrometheusServer, error) {
	pe, err := promexp.NewExporter(promexp.Options{
		Namespace:  appName,
		Registerer: prom.DefaultRegisterer,
		Gatherer:   prom.DefaultGatherer,
	})
	if err != nil {
		return nil, fmt.Errorf("new prometheus exporter: %w", err)
	}

	// register prometheus with opencensus
	view.RegisterExporter(pe)
	view.SetReportingPeriod(2 * time.Second)
	return &PrometheusServer{
		addr: addr,
		pe:   pe,
	}, nil
}

func (p *PrometheusServer) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", p.pe)
	server := &http.Server{Addr: p.addr, Handler: mux}
	go func() {
		select {
		case <-ctx.Done():
			server.Shutdown(context.Background())
		}
	}()

	return server.ListenAndServe()
}

func newCounterMetric(name string, help string, labels []string) (prometheus.Counter, error) {
	m := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "thunderdome",
			Subsystem: "dealgood",
			Name:      name,
			Help:      help,
		},
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(prometheus.Counter)
		} else {
			return nil, fmt.Errorf("register %s counter: %w", name, err)
		}
	}
	return m, nil
}
