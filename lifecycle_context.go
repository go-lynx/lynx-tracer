package tracer

import (
	"context"
	"fmt"

	"github.com/go-lynx/lynx/plugins"
)

func (t *PlugTracer) InitializeContext(ctx context.Context, plugin plugins.Plugin, rt plugins.Runtime) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tracer initialize canceled before execution: %w", err)
	}
	return t.BasePlugin.Initialize(plugin, rt)
}

func (t *PlugTracer) StartContext(ctx context.Context, _ plugins.Plugin) error {
	return t.startupWithContext(ctx)
}

func (t *PlugTracer) StopContext(ctx context.Context, _ plugins.Plugin) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tracer stop canceled before execution: %w", err)
	}
	return t.cleanupWithContext(ctx)
}

func (t *PlugTracer) IsContextAware() bool {
	return true
}
