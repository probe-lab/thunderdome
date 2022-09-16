package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	promexp "contrib.go.opencensus.io/exporter/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"go.opencensus.io/stats/view"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
)

type PrometheusServer struct {
	addr string
	pe   *promexp.Exporter
}

func NewPrometheusServer(addr string) (*PrometheusServer, error) {
	pe, err := promexp.NewExporter(promexp.Options{
		Namespace:  appName,
		Registerer: prometheus.DefaultRegisterer,
		Gatherer:   prometheus.DefaultGatherer,
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

func newPrometheusCounter(name string, help string, labels map[string]string) (prometheus.Counter, error) {
	m := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace:   "thunderdome",
			Subsystem:   appName,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
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

func newPrometheusGauge(name string, help string, labels map[string]string) (prometheus.Gauge, error) {
	m := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   "thunderdome",
			Subsystem:   appName,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		},
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(prometheus.Gauge)
		} else {
			return nil, fmt.Errorf("register %s gauge: %w", name, err)
		}
	}
	return m, nil
}

func newHistogramMetric(name string, help string, labels []string) (*prometheus.HistogramVec, error) {
	m := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "thunderdome",
			Subsystem: "dealgood",
			Name:      name,
			Help:      help,
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 240},
		},
		labels,
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			return nil, fmt.Errorf("register %s histogram: %w", name, err)
		}
	}
	return m, nil
}

func newCounterMetric(name string, help string, labels []string) (*prometheus.CounterVec, error) {
	m := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "thunderdome",
			Subsystem: "dealgood",
			Name:      name,
			Help:      help,
		},
		labels,
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(*prometheus.CounterVec)
		} else {
			return nil, fmt.Errorf("register %s counter: %w", name, err)
		}
	}
	return m, nil
}

func newGaugeMetric(name string, help string, labels []string) (*prometheus.GaugeVec, error) {
	m := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "thunderdome",
			Subsystem: "dealgood",
			Name:      name,
			Help:      help,
		},
		labels,
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			return nil, fmt.Errorf("register %s gauge: %w", name, err)
		}
	}
	return m, nil
}

func setupTracing(ctx context.Context) error {
	tc := propagation.TraceContext{}
	otel.SetTextMapPropagator(tc)

	exporters, err := buildTracerExporters(ctx)
	if err != nil {
		return err
	}

	options := []trace.TracerProviderOption{}

	for _, exporter := range exporters {
		options = append(options, trace.WithBatcher(exporter))
	}

	tp := trace.NewTracerProvider(options...)
	otel.SetTracerProvider(tp)

	return nil
}

func buildTracerExporters(ctx context.Context) ([]trace.SpanExporter, error) {
	var exporters []trace.SpanExporter

	if os.Getenv("OTEL_TRACES_EXPORTER") == "" {
		return exporters, nil
	}

	for _, exporterStr := range strings.Split(os.Getenv("OTEL_TRACES_EXPORTER"), ",") {
		switch exporterStr {
		case "otlp":
			exporter, err := otlptracegrpc.New(ctx)
			if err != nil {
				return nil, fmt.Errorf("new OTLP gRPC exporter: %w", err)
			}
			exporters = append(exporters, exporter)
		case "jaeger":
			exporter, err := jaeger.New(jaeger.WithCollectorEndpoint())
			if err != nil {
				return nil, fmt.Errorf("new Jaeger exporter: %w", err)
			}
			exporters = append(exporters, exporter)
		case "zipkin":
			exporter, err := zipkin.New("")
			if err != nil {
				return nil, fmt.Errorf("new Zipkin exporter: %w", err)
			}
			exporters = append(exporters, exporter)
		default:
			return nil, fmt.Errorf("unknown or unsupported exporter: %q", exporterStr)
		}
	}
	return exporters, nil
}
