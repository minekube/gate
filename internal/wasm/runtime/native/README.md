# Native WebAssembly Runtime Feasibility Spike

This directory proves Gate can host a WebAssembly Component through Wasmtime
and a statically linked Rust library. It is an implementation spike, not yet
the production plugin loader or generated Gate API.

## Prerequisites

- Go is selected from the repository's `go.mod`.
- Rust is selected by the repository's `rust-toolchain.toml`.
- A platform C toolchain supported by CGO is required.
- No global `cargo-component`, `wasm-tools`, or `wit-bindgen` CLI is required.

Run the complete local spike check from the repository root:

```sh
make wasm-native-test
```

That target builds the Rust guest for `wasm32-unknown-unknown`, componentizes
it with the pinned in-workspace tool, builds the Wasmtime host as a static
library, and runs the tagged Go integration tests with CGO enabled.

Normal Gate development remains independent of Rust:

```sh
go test ./...
```

Without the `wasm_native` build tag, the package reports `ErrUnavailable` and
does not link or invoke the Rust runtime.

The release build still uses its existing `CGO_ENABLED=0` configuration. Its
conversion is intentionally deferred until this feasibility result is
accepted and the generated full-API runtime is designed.
