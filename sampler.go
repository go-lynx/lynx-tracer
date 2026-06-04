package tracer

import (
	"github.com/go-lynx/lynx-tracer/conf"
	traceSdk "go.opentelemetry.io/otel/sdk/trace"
)

// buildSampler builds an OpenTelemetry Sampler based on Tracer configuration.
// Priority is given to the modular sampler configuration:
// - ALWAYS_ON:      Always sample (AlwaysSample)
// - ALWAYS_OFF:     Never sample (NeverSample)
// - TRACEID_RATIO:  Sample by ratio (TraceIDRatioBased)
// - PARENT_BASED_TRACEID_RATIO: Parent-based ratio sampling (ParentBased + TraceIDRatioBased)
// Fallback strategy:
// - When config.sampler is not configured or type is not specified, use the outer legacy ratio field, default ParentBased(TraceIDRatioBased(ratio)).
func buildSampler(c *conf.Tracer) traceSdk.Sampler {
	cfg := c.GetConfig()

	// No modular sampler configured: fall back to the legacy top-level ratio for backward
	// compatibility, wrapped in ParentBased so child spans honor the parent's sampling decision.
	if cfg == nil || cfg.Sampler == nil || cfg.Sampler.Type == conf.Sampler_SAMPLER_UNSPECIFIED {
		ratio := clampRatio(c.GetRatio())
		return traceSdk.ParentBased(traceSdk.TraceIDRatioBased(float64(ratio)))
	}

	s := cfg.GetSampler()

	switch s.GetType() {
	case conf.Sampler_ALWAYS_ON:
		return traceSdk.AlwaysSample()

	case conf.Sampler_ALWAYS_OFF:
		return traceSdk.NeverSample()

	case conf.Sampler_TRACEID_RATIO:
		r := s.GetRatio()
		if !isValidRatio(r) {
			r = clampRatio(c.GetRatio()) // fall back to the legacy ratio when the sampler ratio is out of range
		}
		return traceSdk.TraceIDRatioBased(float64(r))

	case conf.Sampler_PARENT_BASED_TRACEID_RATIO:
		r := s.GetRatio()
		if !isValidRatio(r) {
			r = clampRatio(c.GetRatio())
		}
		return traceSdk.ParentBased(traceSdk.TraceIDRatioBased(float64(r)))

	default:
		ratio := clampRatio(c.GetRatio())
		return traceSdk.ParentBased(traceSdk.TraceIDRatioBased(float64(ratio)))
	}
}

// isValidRatio checks if a ratio value is within the valid range [0.0, 1.0]
// Uses epsilon comparison to handle floating-point precision issues
func isValidRatio(ratio float32) bool {
	const epsilon = 1e-9
	return ratio >= -epsilon && ratio <= 1.0+epsilon
}

// clampRatio ensures a ratio value is within the valid range [0.0, 1.0]
func clampRatio(ratio float32) float32 {
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}
