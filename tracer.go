// Package tracer implements an OpenTelemetry distributed tracing plugin for the Lynx framework.
// It manages the full lifecycle of an OTLP-based TracerProvider, including gRPC/HTTP exporter
// construction, sampler configuration, resource attribution, and graceful shutdown with span flushing.
// Prometheus metrics for the plugin's own health (initialization state, startup/shutdown counters,
// and health check outcomes) are exposed via MetricsGatherer so the framework can merge them
// into the shared /metrics endpoint.
package tracer

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/go-lynx/lynx-tracer/conf"
	"github.com/go-lynx/lynx/log"
	"github.com/go-lynx/lynx/plugins"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Plugin metadata, defines basic information of Tracer plugin
const (
	// pluginName is the unique identifier of Tracer plugin in Lynx plugin system.
	pluginName = "tracer.server"

	// pluginVersion represents the current version of Tracer plugin.
	pluginVersion = "v1.6.1"

	// pluginDescription briefly describes the purpose of Tracer plugin.
	pluginDescription = "OpenTelemetry tracer plugin for Lynx framework"

	// confPrefix is the configuration prefix used when loading Tracer configuration.
	confPrefix = "lynx.tracer"
)

// PlugTracer implements the Tracer plugin functionality for Lynx framework.
// It embeds plugins.BasePlugin to inherit common plugin functionality and maintains Tracer tracing configuration and instances.
type PlugTracer struct {
	// Embed base plugin, inheriting common plugin properties and methods
	*plugins.BasePlugin
	// Tracer configuration information (supports modular configuration and backward-compatible old fields)
	conf *conf.Tracer
	// Runtime handle for publishing lifecycle contract resources.
	rt plugins.Runtime
	// tp holds the TracerProvider created by this plugin; used for shutdown to avoid closing a different provider set elsewhere.
	tp *trace.TracerProvider
	// propagator keeps the installed text map propagator available as a runtime resource.
	propagator propagation.TextMapPropagator
	// metrics exposes Prometheus counters and gauges for this plugin's own lifecycle health.
	metrics *TracerMetrics
}

// NewPlugTracer creates a new Tracer plugin instance.
// This function initializes the plugin's basic information (ID, name, description, version, configuration prefix, weight) and returns the instance.
func NewPlugTracer() *PlugTracer {
	return &PlugTracer{
		BasePlugin: plugins.NewBasePlugin(
			plugins.GeneratePluginID("", pluginName, pluginVersion),
			pluginName,
			pluginDescription,
			pluginVersion,
			confPrefix,
			9999, // load weight; high so tracing initializes before most plugins
		),
		conf:    &conf.Tracer{},
		metrics: newTracerMetrics(),
	}
}

// InitializeResources loads and validates Tracer configuration from runtime, while filling default values.
// - First scan "lynx.tracer" from runtime configuration tree to t.conf
// - Validate necessary parameters (sampling ratio range, enabled but unconfigured address, etc.)
// - Set reasonable default values (addr, ratio)
func (t *PlugTracer) InitializeResources(rt plugins.Runtime) error {
	if err := t.BasePlugin.InitializeResources(rt); err != nil {
		return err
	}
	t.rt = rt
	t.conf = &conf.Tracer{}

	err := rt.GetConfig().Value(confPrefix).Scan(t.conf)
	if err != nil {
		return fmt.Errorf("failed to load tracer configuration: %w", err)
	}

	if err := t.validateConfiguration(); err != nil {
		return fmt.Errorf("tracer configuration validation failed: %w", err)
	}

	t.setDefaultValues()

	return nil
}

// validateConfiguration validates configuration legality:
// - ratio must be in [0,1]
// - when enable=true, valid addr must be provided
// - additional validation for config fields when present
func (t *PlugTracer) validateConfiguration() error {
	// Validate sampling ratio (even when disabled, ratio should be valid for future use)
	if t.conf.Ratio < 0 || t.conf.Ratio > 1 {
		return fmt.Errorf("sampling ratio must be between 0 and 1, got %f", t.conf.Ratio)
	}

	// Validate address configuration when tracing is enabled
	if t.conf.Enable && t.conf.Addr == "" {
		return fmt.Errorf("tracer address is required when tracing is enabled")
	}
	// Validate address format when addr is set and not the special "None" (no-export) value
	if t.conf.Enable && t.conf.Addr != "" && t.conf.Addr != "None" {
		if err := validateAddress(t.conf.Addr); err != nil {
			return fmt.Errorf("tracer address validation failed: %w", err)
		}
	}

	// Additional validation for config fields when present
	if t.conf.Config != nil {
		if err := t.validateConfigFields(); err != nil {
			return fmt.Errorf("config validation failed: %w", err)
		}
	}

	return nil
}

// validateConfigFields validates the nested config fields
func (t *PlugTracer) validateConfigFields() error {
	cfg := t.conf.Config

	// Validate batch configuration
	if cfg.Batch != nil && cfg.Batch.GetEnabled() {
		if cfg.Batch.GetMaxQueueSize() < 0 {
			return fmt.Errorf("batch max_queue_size must be non-negative")
		}
		if cfg.Batch.GetMaxBatchSize() < 0 {
			return fmt.Errorf("batch max_batch_size must be non-negative")
		}
		// Validate batch size vs queue size relationship when both are set
		if cfg.Batch.GetMaxBatchSize() > 0 && cfg.Batch.GetMaxQueueSize() > 0 {
			if cfg.Batch.GetMaxBatchSize() > cfg.Batch.GetMaxQueueSize() {
				return fmt.Errorf("batch max_batch_size cannot exceed max_queue_size")
			}
		}
		// When both are 0, buildExporter will apply SDK defaults (max_queue_size=2048, max_batch_size=512)
	}

	// Validate retry configuration
	if cfg.Retry != nil && cfg.Retry.GetEnabled() {
		if cfg.Retry.GetMaxAttempts() < 1 {
			return fmt.Errorf("retry max_attempts must be at least 1")
		}
		if cfg.Retry.GetInitialInterval() != nil && cfg.Retry.GetInitialInterval().AsDuration() < 0 {
			return fmt.Errorf("retry initial_interval must be non-negative")
		}
		if cfg.Retry.GetMaxInterval() != nil && cfg.Retry.GetMaxInterval().AsDuration() < 0 {
			return fmt.Errorf("retry max_interval must be non-negative")
		}
		// Validate interval relationship
		if cfg.Retry.GetInitialInterval() != nil && cfg.Retry.GetMaxInterval() != nil {
			if cfg.Retry.GetInitialInterval().AsDuration() > cfg.Retry.GetMaxInterval().AsDuration() {
				return fmt.Errorf("retry initial_interval cannot exceed max_interval")
			}
		}
	}

	// Validate sampler configuration
	if cfg.Sampler != nil {
		if cfg.Sampler.GetRatio() < 0 || cfg.Sampler.GetRatio() > 1 {
			return fmt.Errorf("sampler ratio must be between 0 and 1, got %f", cfg.Sampler.GetRatio())
		}
	}

	// Validate connection management configuration
	if cfg.Connection != nil {
		if err := t.validateConnectionConfig(cfg.Connection); err != nil {
			return fmt.Errorf("connection configuration validation failed: %w", err)
		}
	}

	// Validate load balancing configuration
	if cfg.LoadBalancing != nil {
		if err := t.validateLoadBalancingConfig(cfg.LoadBalancing); err != nil {
			return fmt.Errorf("load balancing configuration validation failed: %w", err)
		}
	}

	return nil
}

// validateConnectionConfig validates connection management configuration
func (t *PlugTracer) validateConnectionConfig(conn *conf.Connection) error {
	// Validate connection timeout
	if conn.GetConnectTimeout() != nil && conn.GetConnectTimeout().AsDuration() < 0 {
		return fmt.Errorf("connection connect_timeout must be non-negative")
	}

	// Validate reconnection period
	if conn.GetReconnectionPeriod() != nil && conn.GetReconnectionPeriod().AsDuration() < 0 {
		return fmt.Errorf("connection reconnection_period must be non-negative")
	}

	// Validate connection age settings
	if conn.GetMaxConnAge() != nil && conn.GetMaxConnAge().AsDuration() < 0 {
		return fmt.Errorf("connection max_conn_age must be non-negative")
	}

	if conn.GetMaxConnIdleTime() != nil && conn.GetMaxConnIdleTime().AsDuration() < 0 {
		return fmt.Errorf("connection max_conn_idle_time must be non-negative")
	}

	if conn.GetMaxConnAgeGrace() != nil && conn.GetMaxConnAgeGrace().AsDuration() < 0 {
		return fmt.Errorf("connection max_conn_age_grace must be non-negative")
	}

	// Validate relationship between connection age and idle time
	if conn.GetMaxConnAge() != nil && conn.GetMaxConnIdleTime() != nil {
		if conn.GetMaxConnAge().AsDuration() < conn.GetMaxConnIdleTime().AsDuration() {
			return fmt.Errorf("connection max_conn_age cannot be less than max_conn_idle_time")
		}
	}

	return nil
}

// validateLoadBalancingConfig validates load balancing configuration
func (t *PlugTracer) validateLoadBalancingConfig(lb *conf.LoadBalancing) error {
	// Validate load balancing policy
	validPolicies := map[string]bool{
		"pick_first":  true,
		"round_robin": true,
		"least_conn":  true,
	}

	if lb.GetPolicy() != "" && !validPolicies[lb.GetPolicy()] {
		return fmt.Errorf("load balancing policy must be one of: pick_first, round_robin, least_conn, got: %s", lb.GetPolicy())
	}

	return nil
}

// setDefaultValues sets default values for unconfigured items:
//   - addr defaults to localhost:4317 (OTLP/gRPC default port)
//   - legacy top-level ratio uses historical zero-value normalization; use config.sampler type ALWAYS_OFF
//     when you need deterministic sampling disable semantics.
func (t *PlugTracer) setDefaultValues() {
	if t.conf.Addr == "" {
		t.conf.Addr = "localhost:4317"
	}
	t.conf.Ratio = normalizeLegacyRatio(t.conf.Ratio)
}

// normalizeLegacyRatio preserves the historical top-level ratio defaulting behavior.
// The legacy proto3 scalar does not carry field presence, so "unset" and explicit 0 collapse
// to the same value. To avoid silently changing that long-standing default to "drop all spans",
// the runtime keeps zero normalized to full sampling. Use config.sampler.type ALWAYS_OFF for
// explicit sampling disable semantics.
func normalizeLegacyRatio(ratio float32) float32 {
	if ratio == 0 {
		return 1.0
	}
	return ratio
}

// StartupTasks completes OpenTelemetry TracerProvider initialization:
// - Build sampler, resource, Span limits
// - Create OTLP exporter (gRPC/HTTP) based on configuration, and choose batch or sync processor
// - Set global TracerProvider and TextMapPropagator
// - Print initialization logs
func (t *PlugTracer) StartupTasks() error {
	return t.startupWithContext(context.Background())
}

func (t *PlugTracer) startupWithContext(ctx context.Context) error {
	if t.conf == nil {
		t.metrics.recordStartup(fmt.Errorf("configuration nil"))
		return fmt.Errorf("tracer configuration is nil")
	}
	if err := ctx.Err(); err != nil {
		t.metrics.recordStartup(err)
		return fmt.Errorf("tracer startup canceled before execution: %w", err)
	}

	if t.rt != nil {
		if err := t.rt.RegisterSharedResource(pluginName, t); err != nil {
			t.publishRuntimeContract(false, false)
			t.metrics.recordStartup(err)
			return fmt.Errorf("failed to register tracer shared resource: %w", err)
		}
		t.registerRuntimePluginAlias()
		if err := t.rt.RegisterPrivateResource("config", t.conf); err != nil {
			log.Warnf("failed to register tracer private config resource: %v", err)
		}
	}

	if !t.conf.Enable {
		t.publishRuntimeContract(false, true)
		t.metrics.setInitialized(false)
		t.metrics.recordStartup(nil)
		return nil
	}
	t.publishRuntimeContract(false, false)

	log.Infof("Initializing tracing component")

	var tracerProviderOptions []trace.TracerProviderOption

	// Sampler
	sampler := buildSampler(t.conf)
	tracerProviderOptions = append(tracerProviderOptions, trace.WithSampler(sampler))

	// Resource
	res := buildResource(t.conf)
	tracerProviderOptions = append(tracerProviderOptions, trace.WithResource(res))

	// Span limits
	if limits := buildSpanLimits(t.conf); limits != nil {
		tracerProviderOptions = append(tracerProviderOptions, trace.WithRawSpanLimits(*limits))
	}

	t.metrics.setSamplingRatio(t.conf.GetRatio())

	// The special addr "None" means trace-context propagation only, with no exporter.
	if t.conf.GetAddr() != "None" {
		exp, batchOpts, useBatch, err := buildExporter(ctx, t.conf)
		if err != nil {
			t.publishRuntimeContract(false, false)
			t.metrics.recordStartup(err)
			return fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
		if useBatch {
			tracerProviderOptions = append(tracerProviderOptions, trace.WithBatcher(exp, batchOpts...))
		} else {
			tracerProviderOptions = append(tracerProviderOptions, trace.WithSyncer(exp))
		}
		// Optional startup health check: warn if collector is unreachable (no config flag, best-effort)
		if err := probeCollectorReachable(t.conf.Addr, 2*time.Second); err != nil {
			log.Warnf("Tracer exporter target may be unreachable: %v (traces will still be generated)", err)
		}
		// Record which export path is active so dashboards can identify it without log parsing.
		if t.conf.GetConfig() != nil && t.conf.GetConfig().GetProtocol() == conf.Protocol_OTLP_HTTP {
			t.metrics.setExporterProtocol("http")
		} else {
			t.metrics.setExporterProtocol("grpc")
		}
	} else {
		t.metrics.setExporterProtocol("none")
	}

	tp := trace.NewTracerProvider(tracerProviderOptions...)
	otel.SetTracerProvider(tp)

	// Hold our own reference so CleanupTasks always shuts down this provider, not a replacement
	// installed later by other code via SetTracerProvider.
	t.tp = tp

	t.propagator = buildPropagator(t.conf)
	otel.SetTextMapPropagator(t.propagator)

	if t.rt != nil {
		if err := t.rt.RegisterPrivateResource("tracer_provider", t.tp); err != nil {
			log.Warnf("failed to register tracer private provider resource: %v", err)
		}
		if t.propagator != nil {
			if err := t.rt.RegisterPrivateResource("propagator", t.propagator); err != nil {
				log.Warnf("failed to register tracer private propagator resource: %v", err)
			}
		}
	}

	if err := t.CheckHealth(); err != nil {
		t.publishRuntimeContract(false, false)
		t.metrics.recordHealthCheck(false)
		t.metrics.recordStartup(err)
		return err
	}
	t.publishRuntimeContract(true, true)
	t.metrics.recordHealthCheck(true)
	t.metrics.setInitialized(true)
	t.metrics.recordStartup(nil)

	log.Infof("Tracing component successfully initialized")
	return nil
}

// MetricsGatherer returns the Prometheus Gatherer for this plugin's own lifecycle metrics.
// Implements the metricsGathererProvider interface so the Lynx framework can include these
// metrics in the shared /metrics endpoint alongside all other plugin metrics.
func (t *PlugTracer) MetricsGatherer() prometheus.Gatherer {
	if t.metrics == nil {
		return nil
	}
	return t.metrics.GetGatherer()
}

// probeCollectorReachable performs a best-effort TCP dial to the OTLP endpoint.
// Used for optional startup health check; returns an error if the address is unreachable.
func probeCollectorReachable(addr string, timeout time.Duration) error {
	if addr == "" {
		return nil
	}
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	_ = conn.Close()
	return nil
}

// CleanupTasks gracefully shuts down the TracerProvider created by this plugin.
// Called by the framework during plugin Stop; flushes pending spans and releases the exporter connection.
func (t *PlugTracer) CleanupTasks() error {
	return t.cleanupWithContext(context.Background())
}

// shutdownTimeout is the default total budget for graceful tracer shutdown (ForceFlush + Shutdown).
// 60 s gives the batch processor enough time to drain a full queue of 2048 spans
// before the framework's own stop deadline (typically 30 s) would otherwise expire.
const shutdownTimeout = 60 * time.Second

func (t *PlugTracer) cleanupWithContext(parentCtx context.Context) error {
	if t.tp == nil {
		t.publishRuntimeContract(false, false)
		t.metrics.setInitialized(false)
		return nil
	}

	if err := parentCtx.Err(); err != nil {
		t.metrics.recordShutdown(err)
		return fmt.Errorf("tracer cleanup canceled before execution: %w", err)
	}

	// Ensure we always have a deadline so ForceFlush+Shutdown cannot block indefinitely.
	// If the caller already imposed a tighter deadline we inherit it; otherwise we apply
	// our own generous budget that covers draining a large span queue.
	ctx := parentCtx
	cancel := func() {}
	if _, ok := parentCtx.Deadline(); !ok {
		ctx, cancel = context.WithTimeout(parentCtx, shutdownTimeout)
	}
	defer cancel()

	t.publishRuntimeContract(false, false)

	// ForceFlush gets at most half the remaining budget so Shutdown always has a chance
	// to cleanly terminate the exporter connection even if the flush takes a long time.
	flushCtx := ctx
	flushCancel := func() {}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 2*time.Second {
			flushCtx, flushCancel = context.WithTimeout(ctx, remaining/2)
		}
	}
	if err := t.tp.ForceFlush(flushCtx); err != nil {
		log.Errorf("Failed to force flush tracer provider: %v", err)
	}
	flushCancel()

	if err := t.tp.Shutdown(ctx); err != nil {
		log.Errorf("Failed to shutdown tracer provider: %v", err)
		t.metrics.recordShutdown(err)
		return fmt.Errorf("failed to shutdown tracer provider: %w", err)
	}
	log.Infof("Tracer provider shutdown successfully")
	t.tp = nil
	t.propagator = nil
	t.metrics.setInitialized(false)
	t.metrics.recordShutdown(nil)
	return nil
}
