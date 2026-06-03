package tracer

import (
	"github.com/prometheus/client_golang/prometheus"
)

// TracerMetrics holds Prometheus metrics tracking the tracer plugin's own lifecycle health.
// It uses a private registry so multiple instances don't collide in tests or multi-tenant setups.
type TracerMetrics struct {
	registry *prometheus.Registry

	// initialized reports whether a TracerProvider is currently live (1) or not (0).
	initialized prometheus.Gauge

	// startupTotal counts how many times startupWithContext has been attempted.
	startupTotal prometheus.Counter

	// startupErrorsTotal counts how many startup attempts resulted in an error.
	startupErrorsTotal prometheus.Counter

	// shutdownTotal counts how many times cleanupWithContext has been called.
	shutdownTotal prometheus.Counter

	// shutdownErrorsTotal counts how many shutdown attempts resulted in an error.
	shutdownErrorsTotal prometheus.Counter

	// healthChecksTotal tracks health check outcomes labelled by status ("healthy" / "unhealthy").
	healthChecksTotal *prometheus.CounterVec

	// samplingRatio exposes the configured sampling ratio so it can be graphed over time.
	samplingRatio prometheus.Gauge
}

// newTracerMetrics creates a fully initialised TracerMetrics backed by a private registry.
func newTracerMetrics() *TracerMetrics {
	reg := prometheus.NewRegistry()

	m := &TracerMetrics{
		registry: reg,

		initialized: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "lynx",
			Subsystem: "tracer",
			Name:      "initialized",
			Help:      "1 if the OpenTelemetry TracerProvider is currently initialised, 0 otherwise.",
		}),
		startupTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "lynx",
			Subsystem: "tracer",
			Name:      "startup_total",
			Help:      "Total number of tracer plugin startup attempts.",
		}),
		startupErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "lynx",
			Subsystem: "tracer",
			Name:      "startup_errors_total",
			Help:      "Total number of tracer plugin startup failures.",
		}),
		shutdownTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "lynx",
			Subsystem: "tracer",
			Name:      "shutdown_total",
			Help:      "Total number of tracer plugin shutdown attempts.",
		}),
		shutdownErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "lynx",
			Subsystem: "tracer",
			Name:      "shutdown_errors_total",
			Help:      "Total number of tracer plugin shutdown failures.",
		}),
		healthChecksTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "lynx",
			Subsystem: "tracer",
			Name:      "health_checks_total",
			Help:      "Total number of tracer health checks by outcome.",
		}, []string{"status"}),
		samplingRatio: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "lynx",
			Subsystem: "tracer",
			Name:      "sampling_ratio",
			Help:      "Configured trace sampling ratio in [0, 1].",
		}),
	}

	reg.MustRegister(
		m.initialized,
		m.startupTotal,
		m.startupErrorsTotal,
		m.shutdownTotal,
		m.shutdownErrorsTotal,
		m.healthChecksTotal,
		m.samplingRatio,
	)

	return m
}

// GetGatherer returns the Prometheus Gatherer so the framework can merge this
// plugin's metrics into the shared /metrics endpoint.
func (m *TracerMetrics) GetGatherer() prometheus.Gatherer {
	if m == nil {
		return nil
	}
	return m.registry
}

func (m *TracerMetrics) recordStartup(err error) {
	if m == nil {
		return
	}
	m.startupTotal.Inc()
	if err != nil {
		m.startupErrorsTotal.Inc()
	}
}

func (m *TracerMetrics) recordShutdown(err error) {
	if m == nil {
		return
	}
	m.shutdownTotal.Inc()
	if err != nil {
		m.shutdownErrorsTotal.Inc()
	}
}

func (m *TracerMetrics) setInitialized(v bool) {
	if m == nil {
		return
	}
	if v {
		m.initialized.Set(1)
	} else {
		m.initialized.Set(0)
	}
}

func (m *TracerMetrics) recordHealthCheck(healthy bool) {
	if m == nil {
		return
	}
	if healthy {
		m.healthChecksTotal.WithLabelValues("healthy").Inc()
	} else {
		m.healthChecksTotal.WithLabelValues("unhealthy").Inc()
	}
}

func (m *TracerMetrics) setSamplingRatio(ratio float32) {
	if m == nil {
		return
	}
	m.samplingRatio.Set(float64(ratio))
}
