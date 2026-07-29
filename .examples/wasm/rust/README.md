# Rust WebAssembly plugin

This example calls the live Gate proxy from `init`, logs through the supplied
context, and subscribes a typed callback to `ReadyEvent`. Its bindings come
directly from Gate's WIT contract through upstream `wit-bindgen`; there is no
Gate Rust SDK.

Install Rust, the core Wasm target, and the standard Component Model CLI:

```sh
rustup target add wasm32-unknown-unknown
cargo install wasm-tools
```

Build and validate the component:

```sh
make check
```

Copy `gate-rust-example.component.wasm` to Gate's `plugins` directory and
enable `config.wasm`:

```yaml
config:
  wasm:
    enabled: true
    directory: plugins
```

`build.rs` reads `wasm/wit/contract.json`, so metadata always contains the
contract hash used to generate the bindings. After updating Gate, replace the
`wasm/wit` bundle, regenerate/recompile through your language's normal build,
and deploy the new component.
