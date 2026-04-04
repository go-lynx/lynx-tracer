# Validation

## Automated baseline

Run from the module root:

```bash
go test ./...
go vet ./...
```

## Sampling semantics to verify

- `lynx.tracer.config.sampler.type: ALWAYS_OFF` is the authoritative way to disable sampling while the plugin remains enabled.
- The legacy top-level `lynx.tracer.ratio` field is a fallback path only.
- `lynx.tracer.ratio: 0` is still normalized to `1.0` by the current runtime because the legacy proto3 scalar does not preserve field presence. That means "unset" and explicit zero are indistinguishable in this compatibility path.

## Operational checks

- Verify `addr: "None"` when you want propagation without exporter initialization.
- Verify OTLP HTTP deployments do not assume `retry`, `connection`, or `load_balancing` blocks will affect runtime behavior.
- Verify production configs that require no tracing use `config.sampler.type: ALWAYS_OFF` instead of top-level `ratio: 0`.
