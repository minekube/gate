# Gate WebAssembly Component plugins

Gate loads language-neutral [WebAssembly Components](https://component-model.bytecodealliance.org/).
A component exports `plugin.metadata` and `plugin.init(context, proxy)`. The
`context` and `proxy` are the same live objects supplied to a native Gate
plugin; WIT resources keep their identity and Gate controls their lifetime.

Gate does not publish or maintain per-language SDKs. Generate bindings from
[`wit/gate.wit`](wit/gate.wit) with the standard Component Model tooling for
your language. The WIT contract is the API.

The release bundle contains:

- `gate.wit`: callable Gate functions, methods, structs, resources, callbacks,
  events, commands, timers, context cancellation/deadlines, and logging.
- `manifest.json`: the lossless mapping from exported Go declarations to WIT.
- `contract.json`: generator, ABI, canonical-model, and WIT hashes.

All exported declarations in Gate's `api/...` and `pkg/...` packages are
statically analyzed. The native Go bootstrap-only `proxy.Plugin` and
`proxy.Plugins` declarations are intentionally excluded because the component
world replaces them.

## Build a plugin

1. Copy the `wit` directory from the Gate release you deploy.
2. Run the standard WIT binding generator for your language.
3. Export `minekube:gate/plugin.metadata` and
   `minekube:gate/plugin.init`.
4. Compile a WebAssembly Component targeting the `gate-plugin` world.
5. Put the resulting `.wasm` component in Gate's configured plugin directory.

The [Rust example](../.examples/wasm/rust/README.md) uses upstream
`wit-bindgen` and `wasm-tools`; it does not depend on a Gate SDK.

Plugin metadata includes the WIT hash from `contract.json`. Wasmtime verifies
the component's imports and exports structurally before Gate calls `init`.
Additive host API changes remain compatible; when a Gate release makes a
breaking contract change, regenerate your language bindings and rebuild the
component.

## Runtime behavior

- Each plugin has an isolated Wasmtime store, memory/fuel/deadline limits, and
  a generation-checked resource table.
- Calls and callbacks for one plugin run in FIFO order. Different plugins are
  isolated from each other.
- Callback re-entry into Gate is synchronous and does not deadlock the plugin
  executor.
- Event handlers receive a borrowed event transaction. Gate commits the whole
  event only when the callback succeeds and rolls it back on a guest error or
  trap.
- Commands, event subscriptions, and timers are owned by the plugin and are
  removed before its store is destroyed.
- A trapping or timed-out callback disables only the failing plugin.

## Gate configuration

WebAssembly plugins are disabled by default:

```yaml
config:
  wasm:
    enabled: true
    directory: plugins
    maxComponentBytes: 67108864
    memoryBytes: 134217728
    transferBytes: 16777216
    fuel: 10000000
    resourceHandles: 65536
    timerLimit: 1024
    initDeadline: 5s
    callbackDeadline: 100ms
```

The directory is relative to Gate's working directory unless configured as an
absolute path. Gate discovers regular `.wasm` files in deterministic filename
order. A plugin can be disabled by the name returned from `plugin.metadata`:

```yaml
config:
  wasm:
    enabled: true
    directory: plugins
    plugins:
      example-plugin:
        disabled: true
```

## Regenerating the contract

Gate maintainers regenerate every host and public artifact in one transaction:

```sh
make wasm-api-generate
make wasm-api-check
```

Do not edit generated files by hand.
