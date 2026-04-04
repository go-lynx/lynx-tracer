# Lynx Tracer Plugin

`lynx-tracer` installs a Lynx runtime plugin that configures the global OpenTelemetry `TracerProvider` and propagators.

## Runtime facts

- Go module: `github.com/go-lynx/lynx-tracer`
- Config prefix: `lynx.tracer`
- Runtime plugin name: `tracer.server`
- Main effect: initialize the global OpenTelemetry provider used by `otel.Tracer(...)`

## Current configuration model

The runtime reads only the nested `lynx.tracer` tree. The runnable example is [`conf/example_config.yml`](./conf/example_config.yml).

Important runtime notes:

- `enable` and `addr` live under `lynx.tracer`
- the exporter protocol is `lynx.tracer.config.protocol`
- valid protocol values are `OTLP_GRPC` and `OTLP_HTTP`
- valid propagator values include `W3C_TRACE_CONTEXT`, `W3C_BAGGAGE`, `B3`, `B3_MULTI`, and `JAEGER`
- `addr: "None"` enables tracing and propagation without creating an exporter
- top-level `ratio` is a legacy fallback; prefer `config.sampler`
- `ratio: 0` is normalized to `1.0`, so use `config.sampler.type: ALWAYS_OFF` if you want no sampling
- `config.retry`, `config.connection`, and `config.load_balancing` apply only to the OTLP gRPC exporter
- the batch processor delay field is `scheduled_delay`, not `batch_timeout`

## Minimal example

```yaml
lynx:
  tracer:
    enable: true
    addr: "otel-collector:4317"
    config:
      protocol: OTLP_GRPC
      insecure: true
      batch:
        enabled: true
      propagators:
        - W3C_TRACE_CONTEXT
        - W3C_BAGGAGE
```

## Usage

Import the module so the plugin can register itself, then use normal OpenTelemetry APIs after Lynx startup:

```go
import (
	"context"

	_ "github.com/go-lynx/lynx-tracer"

	"go.opentelemetry.io/otel"
)

func traceWork() {
	tracer := otel.Tracer("order-service")
	_, span := tracer.Start(context.Background(), "CreateOrder")
	defer span.End()
}
```

## Operational guidance

- Prefer OTLP gRPC in production when you need retry and connection-management controls.
- For OTLP HTTP, use `config.http_path` and do not expect `config.retry` or `config.connection` to change exporter behavior.
- If you configure `config.batch.enabled: true` and omit queue or batch size, the plugin falls back to the OpenTelemetry SDK defaults.
