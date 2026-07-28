# Native WebAssembly Runtime

This directory proves Gate can host a WebAssembly Component through Wasmtime
and a statically linked Rust library. The generated Gate contract lives in
[`../../api/gate.wit`](../../api/gate.wit).

WIT is the language-neutral plugin interface. Gate does not maintain separate
Go, Rust, TypeScript, or other language SDKs. Plugin authors use their
language's standard WebAssembly Component Model binding generator against
`gate.wit`, then implement the exported `init(context, proxy)` function. The
borrowed context and proxy provide the same Gate API surface exposed to native
plugin initialization.

The contract is generated statically from Gate's public Go packages:

```sh
make wasm-api-generate
make wasm-api-check
```

Additive contract changes remain structurally compatible. After a breaking
Gate API change, a WebAssembly plugin author must regenerate bindings against
the new WIT contract and rebuild the component; Gate does not carry
hand-maintained compatibility shims.

## Prerequisites

- Go is selected from the repository's `go.mod`.
- Rust is selected by the repository's `rust-toolchain.toml`.
- A platform C toolchain supported by CGO is required.
- No global `cargo-component`, `wasm-tools`, or `wit-bindgen` CLI is required.

On Windows, Go's CGO toolchain uses MinGW, so the matching pinned Rust GNU
toolchain must also be installed:

```powershell
rustup toolchain install 1.94.0-x86_64-pc-windows-gnu --profile minimal --target wasm32-unknown-unknown
```

The Make target selects that toolchain automatically on Windows.

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
