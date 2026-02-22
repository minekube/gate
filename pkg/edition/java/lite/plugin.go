package lite

import "context"

// Plugins is used to register Lite plugins with Gate Lite mode.
// Plugins are initialized only when Lite mode is enabled.
var Plugins []Plugin

// Plugin provides a minimal initialization hook for Gate Lite mode extensions.
type Plugin struct {
	Name string
	Init func(ctx context.Context, rt *Runtime) error
}
