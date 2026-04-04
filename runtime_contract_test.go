package tracer

import (
	"testing"

	"github.com/go-lynx/lynx-tracer/conf"
	"github.com/go-lynx/lynx/plugins"
)

func TestTracerRuntimeContract_LocalLifecycle(t *testing.T) {
	base := plugins.NewSimpleRuntime()
	rt := base.WithPluginContext(pluginName)

	tracer := NewPlugTracer()
	tracer.rt = rt
	tracer.conf = &conf.Tracer{
		Enable: true,
		Addr:   "None",
		Ratio:  1,
	}

	if err := tracer.StartupTasks(); err != nil {
		t.Fatalf("StartupTasks failed: %v", err)
	}

	if alias, err := base.GetSharedResource(sharedPluginResourceName); err != nil || alias != tracer {
		t.Fatalf("unexpected shared plugin alias: value=%#v err=%v", alias, err)
	}
	if readiness, err := base.GetSharedResource(sharedReadinessResourceName); err != nil || readiness != true {
		t.Fatalf("unexpected shared readiness: value=%#v err=%v", readiness, err)
	}
	if health, err := base.GetSharedResource(sharedHealthResourceName); err != nil || health != true {
		t.Fatalf("unexpected shared health: value=%#v err=%v", health, err)
	}
	if _, err := rt.GetPrivateResource("config"); err != nil {
		t.Fatalf("private config resource missing: %v", err)
	}
	if _, err := rt.GetPrivateResource("tracer_provider"); err != nil {
		t.Fatalf("private tracer_provider resource missing: %v", err)
	}
	if _, err := rt.GetPrivateResource("propagator"); err != nil {
		t.Fatalf("private propagator resource missing: %v", err)
	}

	if err := tracer.CleanupTasks(); err != nil {
		t.Fatalf("CleanupTasks failed: %v", err)
	}

	if readiness, err := base.GetSharedResource(sharedReadinessResourceName); err != nil || readiness != false {
		t.Fatalf("unexpected shared readiness after cleanup: value=%#v err=%v", readiness, err)
	}
	if health, err := base.GetSharedResource(sharedHealthResourceName); err != nil || health != false {
		t.Fatalf("unexpected shared health after cleanup: value=%#v err=%v", health, err)
	}
}
