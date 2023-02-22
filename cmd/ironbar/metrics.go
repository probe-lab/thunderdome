package main

import (
	"context"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	prom "github.com/prometheus/client_golang/prometheus"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var eventTypeTag, _ = tag.NewKey("event_type")

func eventTypeContext(ctx context.Context, name string) context.Context {
	ctx, _ = tag.New(ctx, tag.Upsert(eventTypeTag, name))
	return ctx
}

func InitMetricReporting(reportingInterval time.Duration) error {
	view.SetReportingPeriod(reportingInterval)

	return nil
}

type Gauge struct {
	*stats.Int64Measure
}

// Set sets the current value of the gauge.
// If there are any tags in the context, measurements will be tagged with them.
func (g *Gauge) Set(ctx context.Context, v int64) {
	stats.Record(ctx, g.M(v))
}

// Add adds a value to the gauge.
// If there are any tags in the context, measurements will be tagged with them.
func (g *Gauge) Add(ctx context.Context, v int64) {
	stats.Record(ctx, g.M(v))
}

func NewDimensionlessGauge(name string, desc string, tagKeys ...tag.Key) (*Gauge, error) {
	m := stats.Int64(name, desc, stats.UnitDimensionless)

	if err := view.Register(&view.View{
		Name:        name,
		Description: desc,
		Measure:     m,
		TagKeys:     tagKeys,
		Aggregation: view.LastValue(),
	}); err != nil {
		return nil, err
	}

	return &Gauge{Int64Measure: m}, nil
}

type Counter struct {
	*stats.Int64Measure
}

// Add adds a value to the counter.
// If there are any tags in the context, measurements will be tagged with them.
func (c *Counter) Add(ctx context.Context, v int64) {
	stats.Record(ctx, c.M(v))
}

func NewDimensionlessCounter(name string, desc string, tagKeys ...tag.Key) (*Counter, error) {
	m := stats.Int64(name, desc, stats.UnitDimensionless)

	if err := view.Register(&view.View{
		Name:        name,
		Description: desc,
		Measure:     m,
		TagKeys:     tagKeys,
		Aggregation: view.Sum(),
	}); err != nil {
		return nil, err
	}

	return &Counter{Int64Measure: m}, nil
}

func RegisterPrometheusExporter(namespace string) (*prometheus.Exporter, error) {
	registry := prom.NewRegistry()
	registry.MustRegister(prom.NewGoCollector(), prom.NewProcessCollector(prom.ProcessCollectorOpts{}))

	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: namespace,
		Registry:  registry,
	})
	if err != nil {
		return nil, err
	}

	view.RegisterExporter(pe)

	return pe, nil
}
