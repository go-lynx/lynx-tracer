package tracer

import (
	"github.com/go-lynx/lynx"
	"github.com/go-lynx/lynx-tracer/conf"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// buildResource builds OpenTelemetry Resource based on Tracer configuration and Lynx application metadata:
// - service.name prefers resource.service_name from config, otherwise falls back to lynx.GetName()
// - Default injection: service.instance.id, service.version, service.namespace
// - Supports additional custom attributes (string key-value pairs)
// Note: Uses Schemaless construction for flexible extension.
func buildResource(c *conf.Tracer) *resource.Resource {
	var r *conf.Resource
	if c != nil && c.GetConfig() != nil {
		r = c.GetConfig().GetResource()
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceInstanceIDKey.String(lynx.GetHost()),
		semconv.ServiceVersionKey.String(lynx.GetVersion()),
	}

	// Safely get namespace from control plane
	if lynx := lynx.Lynx(); lynx != nil {
		if cp := lynx.GetControlPlane(); cp != nil {
			attrs = append(attrs, semconv.ServiceNamespaceKey.String(cp.GetNamespace()))
		}
	}

	// service.name: prefer config override, with validation
	serviceName := lynx.GetName()
	if r != nil && r.GetServiceName() != "" {
		serviceName = r.GetServiceName()
	}

	// Validate service name is not empty
	if serviceName == "" {
		serviceName = "unknown-service" // Provide a fallback service name
	}

	attrs = append(attrs, semconv.ServiceNameKey.String(serviceName))

	// extra attributes
	if r != nil {
		for k, v := range r.GetAttributes() {
			// Validate attribute key and value
			if k != "" && v != "" {
				attrs = append(attrs, attribute.String(k, v))
			}
		}
	}

	return resource.NewSchemaless(attrs...)
}
