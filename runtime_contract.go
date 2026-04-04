package tracer

import "github.com/go-lynx/lynx/log"

const (
	sharedPluginResourceName     = pluginName + ".plugin"
	sharedReadinessResourceName  = pluginName + ".readiness"
	sharedHealthResourceName     = pluginName + ".health"
	privateReadinessResourceName = "readiness"
	privateHealthResourceName    = "health"
)

func (t *PlugTracer) registerRuntimePluginAlias() {
	if t == nil || t.rt == nil {
		return
	}
	if err := t.rt.RegisterSharedResource(sharedPluginResourceName, t); err != nil {
		log.Warnf("failed to register tracer shared plugin alias: %v", err)
	}
}

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
