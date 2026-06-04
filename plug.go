package tracer

import (
	"github.com/go-lynx/lynx/pkg/factory"
	"github.com/go-lynx/lynx/plugins"
)

// init registers the tracer plugin with the global factory so it is constructed
// automatically when configuration under confPrefix is present.
func init() {
	factory.GlobalTypedFactory().RegisterPlugin(pluginName, confPrefix, func() plugins.Plugin {
		return NewPlugTracer()
	})
}
