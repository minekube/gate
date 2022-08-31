package proxy

// Plugins is used to register plugins with the proxy.
// The plugin's init hook is run after the proxy is initialized and
// before serving any connections.
//
// If one init hook errors, the proxy cancels the boot and shuts down;
// will fire the ShutdownEvent so other plugins can gracefully de-initialize.
var Plugins []Plugin

// Plugin provides the ability to extend Gate with external code.
//
// Quick notes on Go's plugin system:
//
// We don't support Go's plugin system as it is not a mature solution.
// They force your plugin implementation to be highly-coupled with Gate's build toolchain,
// the end-result would be very brittle, hard to maintain and the overhead would
// be much higher if the plugin author does not have any control over new versions of Gate.
//
// Now with that made clear, here is how Gate can be customized!
//
// Native Go:
//
// # You can use Gate as a framework and compile your code with it.
//   - Create your own Go project/module and `go get -u go.minekube.com/gate`
//   - Within your main function
//   - Add your Plugin to the Plugins
//   - And call cmd/gate.Execute (blocking your main until shutdown).
//   - Subscribe to proxy.ShutdownEvent for de-initializing your plugin.
//
// By running cmd/gate.Execute, Gate will do the whole rest.
//   - load the cfg (parse found file, flags and env vars)
//   - make and run the proxy.Proxy that will call the Plugins init hooks.
//
// Script languages:
//   - Not yet supported.
type Plugin struct {
	Name string                   // The name identifying the plugin.
	Init func(proxy *Proxy) error // The hook to initialize the plugin.
}
