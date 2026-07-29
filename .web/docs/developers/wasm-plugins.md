---
title: WebAssembly Plugins
description: Build language-neutral Gate plugins with WebAssembly Components and the generated Gate WIT contract.
---

# WebAssembly plugins

Gate can load plugins written in any language that produces a
[WebAssembly Component](https://component-model.bytecodealliance.org/).
The guest exports the `minekube:gate/plugin` interface, and Gate calls:

```wit
init: func(context: borrow<context>, proxy: borrow<proxy>)
  -> result<_, gate-error>;
```

This is the component equivalent of Gate's native plugin `Init`: it receives
the live plugin context and proxy, not JSON snapshots. Generated WIT resources
preserve object identity, methods remain callable, and Gate owns borrowed
lifetimes.

## WIT is the language-neutral API

WebAssembly bytecode is language-neutral, but source code still needs bindings
in its own language. Gate publishes one WIT contract rather than maintaining
separate Go, Rust, or TypeScript SDKs. Use the standard Component Model binding
generator for your language against `gate.wit`.

Each Gate release includes:

- `gate.wit`, the callable component contract.
- `manifest.json`, the exact Go-to-WIT declaration and type mapping.
- `contract.json`, the ABI and contract hashes.

The same files live in the
[`wasm/wit`](https://github.com/minekube/gate/tree/master/wasm/wit)
directory; the larger manifest is kept with Gate's
[`generated host artifacts`](https://github.com/minekube/gate/tree/master/internal/builtin/wasm/generated)
and included in release bundles. The contract is generated statically from all
exported declarations under Gate's `api/...` and `pkg/...` packages. Only the
native Go plugin bootstrap types are excluded because the component world
replaces them.

Gate does not adapt mismatched binaries at runtime. If a Gate update changes
the WIT contract incompatibly, replace the contract bundle, regenerate
bindings with your language's normal toolchain, rebuild, and redeploy the
plugin. Wasmtime checks imports and exports structurally before Gate calls
`init`, so additive host API changes do not force a rebuild.

## Rust example

The
[`gate-wasm-rust-example`](https://github.com/minekube/gate/tree/master/.examples/wasm/rust)
uses upstream `wit-bindgen` directly. It:

- exports plugin metadata and `init`;
- calls `Proxy.PlayerCount` through the live proxy resource;
- logs through the supplied context;
- subscribes a typed callback to `ReadyEvent`; and
- derives its metadata hash from `contract.json` at build time.

Build it with:

```sh
rustup target add wasm32-unknown-unknown
cargo install wasm-tools
cd .examples/wasm/rust
make check
```

The output is `gate-rust-example.component.wasm`.

## Enable plugins

Copy component `.wasm` files into the configured directory and enable the
runtime:

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

Gate discovers regular `.wasm` files in deterministic filename order.
Individual components can be disabled by the metadata name they export:

```yaml
config:
  wasm:
    enabled: true
    directory: plugins
    plugins:
      gate-rust-example:
        disabled: true
```

## Events, commands, and timers

The generated contract exposes typed callbacks in both directions.

- Event subscriptions use the same event types and priority ordering as Gate.
  The callback gets a borrowed transaction. All mutations commit on success;
  a returned error or trap rolls back the whole event.
- `pkg-command.wasm-register-command` registers a command plus aliases and
  returns an unregister callback.
- `pkg-gate.wasm-after` and `pkg-gate.wasm-every` schedule one-shot and
  non-overlapping recurring callbacks.
- `pkg-gate.wasm-context-*` exposes cancellation, deadline, and error state.
- `pkg-gate.wasm-log` writes through the logger in the supplied context.

Plugin-owned registrations are removed before the component store closes.

## Isolation and limits

Each component has its own Wasmtime store, linear-memory limit, fuel budget,
callback deadlines, transfer-size limit, and generation-checked resource
table. Calls for one plugin are serialized in FIFO order while different
plugins remain isolated. A trap or deadline failure disables only the failing
plugin.
