// Package sysevent provides internal system event types.
package sysevent

// ConfigReloadedEvent is fired when the config is reloaded.
type ConfigReloadedEvent[T any] struct {
	// Config is the new config.
	Config *T
}
