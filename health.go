package tracer

import "fmt"

// CheckHealth verifies that the tracer runtime has been initialized when tracing is enabled.
func (t *PlugTracer) CheckHealth() error {
	if t.conf == nil {
		return fmt.Errorf("tracer configuration is nil")
	}
	if !t.conf.Enable {
		return nil
	}
	if t.tp == nil {
		return fmt.Errorf("tracer provider is not initialized")
	}
	if t.conf.Addr == "" {
		return fmt.Errorf("tracer address is not configured")
	}
	return nil
}
