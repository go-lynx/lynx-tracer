package tracer

import "github.com/go-lynx/lynx/log"

const (
	sharedPluginResourceName     = pluginName + ".plugin"
	sharedReadinessResourceName  = pluginName + ".readiness"
	sharedHealthResourceName     = pluginName + ".health"
	privateReadinessResourceName = "readiness"
	privateHealthResourceName    = "health"
)

// registerRuntimePluginAlias publishes the PlugTracer instance itself as a shared resource
// under the canonical alias key (pluginName + ".plugin").  Other plugins that need a typed
// handle to the tracer can look it up by that key.  The operation is idempotent: repeated
// calls simply overwrite the previous value with the same pointer.
func (t *PlugTracer) registerRuntimePluginAlias() {
	if t == nil || t.rt == nil {
		return
	}
	if err := t.rt.RegisterSharedResource(sharedPluginResourceName, t); err != nil {
		log.Warnf("failed to register tracer shared plugin alias: %v", err)
	}
}

// publishRuntimeContract updates the shared and private readiness/health resources so the
// rest of the application can observe the tracer's current lifecycle state without importing
// this package.  Callers invoke it at every meaningful state transition (startup, ready,
// shutdown).  The underlying RegisterSharedResource / RegisterPrivateResource calls are
// idempotent: a second call with the same key overwrites the previous value.
func (t *PlugTracer) publishRuntimeContract(ready, healthy bool) {
	if t == nil || t.rt == nil {
		return
	}
	for _, item := range []struct {
		name  string
		value any
	}{
		{name: sharedReadinessResourceName, value: ready},
		{name: sharedHealthResourceName, value: healthy},
	} {
		if err := t.rt.RegisterSharedResource(item.name, item.value); err != nil {
			log.Warnf("failed to register tracer shared runtime contract %s: %v", item.name, err)
		}
	}
	if err := t.rt.RegisterPrivateResource(privateReadinessResourceName, ready); err != nil {
		log.Warnf("failed to register tracer private readiness resource: %v", err)
	}
	if err := t.rt.RegisterPrivateResource(privateHealthResourceName, healthy); err != nil {
		log.Warnf("failed to register tracer private health resource: %v", err)
	}
}
