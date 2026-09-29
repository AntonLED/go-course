//go:build !solution

package metrics

import (
	"io"
	"net/http"
)

// Registry хранит метрики и отдаёт их в текстовом формате Prometheus.
type Registry struct {
	// TODO
}

func NewRegistry() *Registry { panic("TODO") }

// NewCounter регистрирует счётчик (только растёт).
func (r *Registry) NewCounter(name, help string, labelNames ...string) (*Counter, error) {
	panic("TODO")
}

// NewGauge регистрирует gauge (произвольное значение).
func (r *Registry) NewGauge(name, help string, labelNames ...string) (*Gauge, error) {
	panic("TODO")
}

// NewHistogram регистрирует гистограмму; buckets == nil → DefBuckets.
func (r *Registry) NewHistogram(name, help string, buckets []float64, labelNames ...string) (*Histogram, error) {
	panic("TODO")
}

// WriteText пишет все метрики в text exposition format (0.0.4).
func (r *Registry) WriteText(w io.Writer) error { panic("TODO") }

// Handler — http.Handler для /metrics.
func (r *Registry) Handler() http.Handler { panic("TODO") }

type Counter struct{}
type CounterSeries struct{}

// WithLabelValues возвращает серию для значений лейблов (создаёт при первом
// обращении). Неверное количество значений — panic.
func (c *Counter) WithLabelValues(values ...string) *CounterSeries { panic("TODO") }
func (s *CounterSeries) Inc()                                      { panic("TODO") }

// Add паникует при v < 0.
func (s *CounterSeries) Add(v float64) { panic("TODO") }

type Gauge struct{}
type GaugeSeries struct{}

func (g *Gauge) WithLabelValues(values ...string) *GaugeSeries { panic("TODO") }
func (s *GaugeSeries) Set(v float64)                           { panic("TODO") }
func (s *GaugeSeries) Add(v float64)                           { panic("TODO") }
func (s *GaugeSeries) Inc()                                    { panic("TODO") }
func (s *GaugeSeries) Dec()                                    { panic("TODO") }

type Histogram struct{}
type HistogramSeries struct{}

func (h *Histogram) WithLabelValues(values ...string) *HistogramSeries { panic("TODO") }
func (s *HistogramSeries) Observe(v float64)                           { panic("TODO") }
