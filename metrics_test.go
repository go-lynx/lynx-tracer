package tracer

import (
	"testing"

	"github.com/go-lynx/lynx-tracer/conf"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTracerMetrics(t *testing.T) {
	m := newTracerMetrics()
	require.NotNil(t, m)
	require.NotNil(t, m.registry)
}

func TestTracerMetrics_GetGatherer(t *testing.T) {
	m := newTracerMetrics()
	g := m.GetGatherer()
	require.NotNil(t, g)

	// Seed all counters so they appear in the gathered output.
	m.recordStartup(nil)
	m.recordShutdown(nil)
	m.recordHealthCheck(true)

	families, err := g.Gather()
	require.NoError(t, err)

	names := make(map[string]struct{})
	for _, f := range families {
		names[f.GetName()] = struct{}{}
	}
	assert.Contains(t, names, "lynx_tracer_initialized")
	assert.Contains(t, names, "lynx_tracer_startup_total")
	assert.Contains(t, names, "lynx_tracer_startup_errors_total")
	assert.Contains(t, names, "lynx_tracer_shutdown_total")
	assert.Contains(t, names, "lynx_tracer_shutdown_errors_total")
	assert.Contains(t, names, "lynx_tracer_health_checks_total")
	assert.Contains(t, names, "lynx_tracer_sampling_ratio")
}

func TestTracerMetrics_GetGatherer_Nil(t *testing.T) {
	var m *TracerMetrics
	assert.Nil(t, m.GetGatherer())
}

func TestTracerMetrics_setInitialized(t *testing.T) {
	m := newTracerMetrics()

	m.setInitialized(true)
	assert.Equal(t, 1.0, getGaugeValue(t, m, "lynx_tracer_initialized"))

	m.setInitialized(false)
	assert.Equal(t, 0.0, getGaugeValue(t, m, "lynx_tracer_initialized"))
}

func TestTracerMetrics_recordStartup(t *testing.T) {
	m := newTracerMetrics()

	m.recordStartup(nil)
	assert.Equal(t, 1.0, getCounterValue(t, m, "lynx_tracer_startup_total"))
	assert.Equal(t, 0.0, getCounterValue(t, m, "lynx_tracer_startup_errors_total"))

	m.recordStartup(assert.AnError)
	assert.Equal(t, 2.0, getCounterValue(t, m, "lynx_tracer_startup_total"))
	assert.Equal(t, 1.0, getCounterValue(t, m, "lynx_tracer_startup_errors_total"))
}

func TestTracerMetrics_recordShutdown(t *testing.T) {
	m := newTracerMetrics()

	m.recordShutdown(nil)
	assert.Equal(t, 1.0, getCounterValue(t, m, "lynx_tracer_shutdown_total"))
	assert.Equal(t, 0.0, getCounterValue(t, m, "lynx_tracer_shutdown_errors_total"))

	m.recordShutdown(assert.AnError)
	assert.Equal(t, 2.0, getCounterValue(t, m, "lynx_tracer_shutdown_total"))
	assert.Equal(t, 1.0, getCounterValue(t, m, "lynx_tracer_shutdown_errors_total"))
}

func TestTracerMetrics_recordHealthCheck(t *testing.T) {
	m := newTracerMetrics()

	m.recordHealthCheck(true)
	m.recordHealthCheck(true)
	m.recordHealthCheck(false)

	families, err := m.registry.Gather()
	require.NoError(t, err)

	var healthy, unhealthy float64
	for _, f := range families {
		if f.GetName() != "lynx_tracer_health_checks_total" {
			continue
		}
		for _, metric := range f.GetMetric() {
			for _, lp := range metric.GetLabel() {
				if lp.GetName() == "status" {
					switch lp.GetValue() {
					case "healthy":
						healthy = metric.GetCounter().GetValue()
					case "unhealthy":
						unhealthy = metric.GetCounter().GetValue()
					}
				}
			}
		}
	}
	assert.Equal(t, 2.0, healthy)
	assert.Equal(t, 1.0, unhealthy)
}

func TestTracerMetrics_setSamplingRatio(t *testing.T) {
	m := newTracerMetrics()
	m.setSamplingRatio(0.5)
	assert.InDelta(t, 0.5, getGaugeValue(t, m, "lynx_tracer_sampling_ratio"), 0.001)
}

func TestTracerMetrics_NilSafe(t *testing.T) {
	var m *TracerMetrics
	// All methods must be safe on nil receiver
	assert.NotPanics(t, func() { m.setInitialized(true) })
	assert.NotPanics(t, func() { m.recordStartup(nil) })
	assert.NotPanics(t, func() { m.recordShutdown(nil) })
	assert.NotPanics(t, func() { m.recordHealthCheck(true) })
	assert.NotPanics(t, func() { m.setSamplingRatio(1.0) })
}

func TestPlugTracer_MetricsGatherer(t *testing.T) {
	p := NewPlugTracer()
	g := p.MetricsGatherer()
	require.NotNil(t, g)

	// Should gather without error
	_, err := g.Gather()
	require.NoError(t, err)
}

func TestPlugTracer_MetricsWiredOnStartup(t *testing.T) {
	p := NewPlugTracer()
	p.conf = &conf.Tracer{Enable: false, Addr: "", Ratio: 1}

	err := p.StartupTasks()
	require.NoError(t, err)

	// Startup should have been recorded
	assert.Equal(t, 1.0, getCounterValue(t, p.metrics, "lynx_tracer_startup_total"))
	assert.Equal(t, 0.0, getCounterValue(t, p.metrics, "lynx_tracer_startup_errors_total"))
	// Disabled tracer: initialized should be 0
	assert.Equal(t, 0.0, getGaugeValue(t, p.metrics, "lynx_tracer_initialized"))
}

// --- helpers ---

func getCounterValue(t *testing.T, m *TracerMetrics, name string) float64 {
	t.Helper()
	families, err := m.registry.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == name {
			if len(f.GetMetric()) > 0 {
				return f.GetMetric()[0].GetCounter().GetValue()
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}

func getGaugeValue(t *testing.T, m *TracerMetrics, name string) float64 {
	t.Helper()
	families, err := m.registry.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == name && f.GetType() == dto.MetricType_GAUGE {
			if len(f.GetMetric()) > 0 {
				return f.GetMetric()[0].GetGauge().GetValue()
			}
		}
	}
	t.Fatalf("gauge metric %q not found", name)
	return 0
}
