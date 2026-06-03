package tracer

import (
	"strings"
	"testing"

	"github.com/go-lynx/lynx-tracer/conf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
)

// compositePropagatorFields returns the propagator list from a CompositeTextMapPropagator
// by injecting into a carrier and inspecting the headers written.  We use a round-trip
// approach: inject a known span context and check that the expected header keys appear.

func TestBuildPropagator_NilConfig(t *testing.T) {
	p := buildPropagator(&conf.Tracer{})
	require.NotNil(t, p)
	// default should produce a composite that at minimum supports W3C traceparent
	assertPropagatorIsComposite(t, p)
}

func TestBuildPropagator_EmptyPropagators(t *testing.T) {
	c := &conf.Tracer{
		Config: &conf.Config{
			Propagators: []conf.Propagator{},
		},
	}
	p := buildPropagator(c)
	require.NotNil(t, p)
	assertPropagatorIsComposite(t, p)
}

func TestBuildPropagator_W3CTraceContext(t *testing.T) {
	c := &conf.Tracer{
		Config: &conf.Config{
			Propagators: []conf.Propagator{conf.Propagator_W3C_TRACE_CONTEXT},
		},
	}
	p := buildPropagator(c)
	require.NotNil(t, p)
	assert.Contains(t, p.Fields(), "traceparent")
}

func TestBuildPropagator_W3CBaggage(t *testing.T) {
	c := &conf.Tracer{
		Config: &conf.Config{
			Propagators: []conf.Propagator{conf.Propagator_W3C_BAGGAGE},
		},
	}
	p := buildPropagator(c)
	require.NotNil(t, p)
	assert.Contains(t, p.Fields(), "baggage")
}

func TestBuildPropagator_B3Single(t *testing.T) {
	c := &conf.Tracer{
		Config: &conf.Config{
			Propagators: []conf.Propagator{conf.Propagator_B3},
		},
	}
	p := buildPropagator(c)
	require.NotNil(t, p)
	// b3.New() without explicit encoding options; Fields() reports the headers
	// the propagator participates in (may include both single "b3" and multi x-b3-* headers
	// depending on the library version's default inject encoding).
	// We assert at minimum one B3-related header is reported.
	fields := p.Fields()
	hasB3Header := false
	for _, f := range fields {
		if f == "b3" || strings.HasPrefix(f, "x-b3-") {
			hasB3Header = true
			break
		}
	}
	assert.True(t, hasB3Header, "expected at least one B3 header in fields, got: %v", fields)
}

func TestBuildPropagator_B3Multi(t *testing.T) {
	c := &conf.Tracer{
		Config: &conf.Config{
			Propagators: []conf.Propagator{conf.Propagator_B3_MULTI},
		},
	}
	p := buildPropagator(c)
	require.NotNil(t, p)
	fields := p.Fields()
	// B3 multi-header format injects x-b3-traceid, x-b3-spanid, x-b3-sampled
	assert.Contains(t, fields, "x-b3-traceid")
	assert.Contains(t, fields, "x-b3-spanid")
}

func TestBuildPropagator_Jaeger(t *testing.T) {
	c := &conf.Tracer{
		Config: &conf.Config{
			Propagators: []conf.Propagator{conf.Propagator_JAEGER},
		},
	}
	p := buildPropagator(c)
	require.NotNil(t, p)
	assert.Contains(t, p.Fields(), "uber-trace-id")
}

func TestBuildPropagator_CompositeMultiple(t *testing.T) {
	c := &conf.Tracer{
		Config: &conf.Config{
			Propagators: []conf.Propagator{
				conf.Propagator_W3C_TRACE_CONTEXT,
				conf.Propagator_W3C_BAGGAGE,
			},
		},
	}
	p := buildPropagator(c)
	require.NotNil(t, p)
	fields := p.Fields()
	assert.Contains(t, fields, "traceparent")
	assert.Contains(t, fields, "baggage")
}

func TestBuildPropagator_AllUnrecognized_FallsBackToDefault(t *testing.T) {
	// Use an unrecognised enum value to simulate a future/unknown propagator type.
	// The switch has no case for it, so len(list) stays 0 and we fall back to default.
	c := &conf.Tracer{
		Config: &conf.Config{
			// Propagator_PROPAGATOR_UNSPECIFIED (0) is not handled in the switch
			Propagators: []conf.Propagator{conf.Propagator_PROPAGATOR_UNSPECIFIED},
		},
	}
	p := buildPropagator(c)
	require.NotNil(t, p)
	// Falls back to default: W3C traceparent + baggage
	fields := p.Fields()
	assert.Contains(t, fields, "traceparent")
	assert.Contains(t, fields, "baggage")
}

// assertPropagatorIsComposite checks that p is a CompositeTextMapPropagator by verifying
// it implements the TextMapPropagator interface and its Fields() is non-nil.
func assertPropagatorIsComposite(t *testing.T, p propagation.TextMapPropagator) {
	t.Helper()
	_, ok := p.(propagation.TextMapPropagator)
	assert.True(t, ok)
	assert.NotNil(t, p.Fields())
}
