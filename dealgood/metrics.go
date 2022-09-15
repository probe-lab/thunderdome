package main

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

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
