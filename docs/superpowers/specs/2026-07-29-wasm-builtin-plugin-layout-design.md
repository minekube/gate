# Wasm Built-In Plugin Layout

Date: 2026-07-29

Status: Approved

## Summary

Gate's WebAssembly component support will remain compiled into the standard
Gate binary, but it will be organized and initialized as a built-in native Gate
plugin. Wasm support is enabled by default. An absent or empty component plugin
directory is a successful no-op.

All implementation details will live in one vertical feature subtree under
`internal/builtin/wasm`. Public WIT contracts, documentation, and examples
remain outside `internal`.

## Goals

- Make the Wasm subsystem's ownership and optional-plugin boundary obvious.
- Use the existing native `proxy.Plugin` initialization lifecycle instead of a
  second component-plugin lifecycle in `proxy.Proxy`.
- Keep Wasm compiled into and enabled by default in the official Gate binary.
- Preserve the existing generated contract, runtime behavior, isolation, and
  fail-fast startup policy.
- Keep the complete subsystem easy to extract into another module later without
  designing a separate module now.

## Non-goals

- Dynamically load native Go plugins.
- Move Wasm support into another repository or Go module.
- Change the published WIT contract or guest authoring experience.
- Add compatibility shims for components built against breaking older
  contracts.
- Refactor unrelated Gate packages.

## Repository Layout

The implementation will use this vertical layout:

```text
internal/builtin/
└── wasm/
    ├── plugin.go
    ├── config.go             # built-in defaults and runtime interpretation
    ├── manager.go
    ├── executor.go
    ├── codegen/
    │   ├── analyze/
    │   ├── model/
    │   ├── generate/
    │   └── cmd/
    ├── generated/
    ├── host/
    ├── runtime/
    │   ├── dispatch/
    │   ├── resources/
    │   └── wire/
    └── wasmtime/
```

The exact package split may combine files where a separate package would add no
useful boundary, but these responsibilities remain distinct:

- `plugin.go` adapts the subsystem to `proxy.Plugin`.
- `config.go` owns built-in defaults and runtime interpretation. The exported
  `pkg/edition/java/config.Wasm` schema remains at its existing public import
  path.
- `manager.go` and `executor.go` discover, initialize, serialize, and close
  components.
- `codegen` owns Go API analysis, the canonical model, and synchronized
  artifact generation.
- `generated` contains generated Go dispatch and contract metadata.
- `host` adapts generated operations to public Gate APIs and Gate-specific
  extensions.
- `runtime` owns generic dispatch, resource tables, and wire values.
- `wasmtime` owns the Rust host, componentization, CGO bridge, and native build
  assets.

Public authoring artifacts remain stable:

```text
wasm/
├── README.md
└── wit/
    ├── gate.wit
    └── contract.json

.examples/wasm/
└── rust/
```

## Lifecycle and Configuration

The built-in package exposes a native Gate plugin constructor:

```go
func Plugin(cfg Config) proxy.Plugin
```

The standard `cmd/gate` assembly registers this plugin automatically. Its
`Init(context.Context, *proxy.Proxy) error` hook receives the same initialized
context and proxy as every other native plugin and runs before Gate accepts
connections.

The built-in is registered after application-supplied native plugins, preserving
the existing order in which native plugin hooks run before component plugin
initialization.

Wasm is enabled by default. The default component directory remains `plugins/`.
A missing directory or a directory containing no components returns success
without allocating a Wasmtime runtime.

An explicit `wasm.enabled: false` setting disables discovery and runtime
initialization while keeping the implementation compiled into the binary.

The built-in plugin registers blocking shutdown cleanup through Gate's native
plugin lifecycle. Components close in reverse initialization order. Partial
startup failure closes every component that initialized successfully before
returning the original startup error.

## Removal of the Parallel Lifecycle

The Wasm implementation will no longer require the
`proxy.ComponentPluginManager` field or the `gate.WithComponentPlugins`
configuration option. `proxy.Proxy` will initialize only native plugins; the
built-in Wasm plugin owns its component manager internally.

This removes Wasm-specific orchestration from `proxy.Proxy` without changing
the lifecycle exposed to component guests:

```text
Gate startup
  -> native built-in Wasm Plugin.Init(ctx, proxy)
  -> discover and instantiate components
  -> component init(context-resource, proxy-resource)
  -> Gate begins accepting connections
```

## Error Policy

These conditions abort Gate startup:

- an unreadable or oversized component;
- invalid component metadata;
- duplicate component identities;
- contract incompatibility;
- Wasmtime compilation or instantiation failure;
- component `init` failure;
- invalid configured resource limits;
- unsupported or stale generated Gate API artifacts.

Missing and empty component directories are not errors.

Runtime traps and limit violations remain isolated to the responsible
component according to the existing runtime policy. Moving packages must not
weaken panic containment, resource ownership, callback serialization, fuel,
memory, transfer, deadline, or handle limits.

## Generation and Compatibility

The generator remains a Gate maintainer tool inside the built-in Wasm subtree.
It continues to:

- load public Gate packages with `go/packages` and `go/types`;
- construct one deterministic canonical API model;
- generate WIT, Go, Rust, C, manifest, and compatibility artifacts from that
  model;
- fail rather than silently omit unsupported public declarations;
- detect generated artifact drift in CI;
- make Gate API drift visible through generated compile-time references.

Moving the generator does not turn it into a guest SDK generator. Component
authors continue using standard WIT tooling for their chosen language.

## Testing

Existing tests move with their owning packages and preserve behavior. The
refactor must retain coverage for:

- default-enabled and explicit-disable configuration;
- missing and empty component directories;
- native plugin initialization ordering and identical context/proxy values;
- startup rollback and reverse-order shutdown;
- component identity and contract validation;
- resource, callback, timer, memory, fuel, transfer, and deadline limits;
- generated dispatcher compile-time references;
- deterministic generation and checked-in artifact drift;
- Rust host and real Wasmtime component integration;
- disabled native-runtime builds where CGO or the target platform does not
  support the embedded host.

The completed refactor must pass the repository's Go tests, generation checks,
Rust tests, linting, and release build checks used by the Wasm feature branch.

## Migration Constraints

- Move files without changing generated public contract contents unless an
  import-path-only internal artifact necessarily changes.
- Preserve the exported `pkg/edition/java/config.Wasm` Go type and its YAML and
  JSON keys.
- Preserve git history through focused moves where practical.
- Update Go imports, `go:generate` directives, Rust build paths, release scripts,
  Docker assets, and CI path filters together.
- Do not leave compatibility aliases for removed internal package paths.
- Keep public configuration keys under `wasm` stable.
