# Gate WebAssembly Generated Dispatcher Plan

> **Execution requirement:** Begin only after the analyzer completion
> checkpoint. Use executing-plans and test-driven-development. Each task starts
> red and ends in a focused commit.

**Goal:** Generate the complete typed Go/C/Rust host bridge from the canonical
Gate API model so a component can call every represented operation and receive
typed callbacks without handwritten per-API wrappers.

**Architecture:** The canonical model drives four synchronized renderers:
production WIT, C ABI, Go dispatchers, and Rust Wasmtime adapters. Operations
have stable numeric IDs for diagnostics and compatibility metadata, but each
operation has generated typed entry points and layouts; there is no JSON or
generic runtime RPC. Identity-bearing Go values remain in a per-plugin Go
resource table and cross FFI only as typed handles.

**Generated layout:**

```text
internal/wasm/api/
  gate.wit
  gate_wasm_generated.h
  dispatch_gen.go
  values_gen.go
  callbacks_gen.go
  manifest.json
  contract.json
internal/wasm/runtime/native/host/src/generated/
  bindings.rs
  dispatch.rs
  values.rs
  callbacks.rs
```

---

### Task 1: Shared generated ABI model

**Files:**

- Create: `internal/wasm/generate/abi.go`
- Create: `internal/wasm/generate/abi_test.go`
- Create: `internal/wasm/runtime/abi/value.go`
- Create: `internal/wasm/runtime/abi/value_test.go`

- [ ] Add failing layout tests for scalars, options, records, variants, lists,
  strings, results, resources, callbacks, and nested combinations.
- [ ] Define owner-specific fixed-width C layouts, explicit discriminants,
  pointer/length/capacity buffers, and typed 64-bit handles. Forbid `uintptr`,
  Go `int`, C `long`, and implicit enum sizes in generated ABI.
- [ ] Include an ABI schema version and layout fingerprint in contract
  metadata. Assert Go and Rust expected size/alignment values for every
  generated layout.
- [ ] Keep borrowed buffers valid only during a call. Every owned output names
  its allocator and generated free operation.
- [ ] Run `go test ./internal/wasm/generate ./internal/wasm/runtime/abi -count=1`.
- [ ] Commit as `feat: define generated wasm ABI layouts`.

---

### Task 2: Per-plugin typed resource table

**Files:**

- Create: `internal/wasm/runtime/resources/table.go`
- Create: `internal/wasm/runtime/resources/table_test.go`
- Create: `internal/wasm/runtime/resources/handle.go`
- Create: `internal/wasm/runtime/resources/handle_test.go`

- [ ] Add failing tests for plugin ownership, resource type, generation,
  explicit drop, borrowed-call expiry, borrowed-event expiry, Gate-owned
  invalidation, capacity, and stale-handle reuse.
- [ ] Encode table slot and generation in the numeric handle while retaining
  plugin and type identity in validated table metadata. A handle never embeds a
  Go pointer.
- [ ] Implement lifetime classes `plugin`, `owned`, `borrowed-call`,
  `borrowed-event`, and `gate-owned`.
- [ ] Reject foreign, mistyped, expired, double-dropped, and over-limit
  operations with stable error kinds.
- [ ] Add leak counters and a table `Close` that invalidates all generations
  and releases Go references exactly once.
- [ ] Run `go test ./internal/wasm/runtime/resources -count=1 -race`.
- [ ] Commit as `feat: isolate wasm plugin resources`.

---

### Task 3: Generated value conversion and ownership

**Files:**

- Modify: `internal/wasm/generate/abi.go`
- Create: `internal/wasm/generate/go_values.go`
- Create: `internal/wasm/generate/rust_values.go`
- Generate: `internal/wasm/api/values_gen.go`
- Generate:
  `internal/wasm/runtime/native/host/src/generated/values.rs`
- Create cross-language fixture tests in
  `internal/wasm/runtime/native/host/tests/generated_values.rs`

- [ ] Add failing fixture tests that round-trip every canonical value shape,
  including nil options, empty/non-empty lists, Unicode, nested records,
  variants, maps as entries, typed errors, timestamps, durations, and dynamic
  values.
- [ ] Generate Go-to-C, C-to-Go, Rust-to-C, and C-to-Rust conversion code from
  the same ABI node. Enforce transfer limits before allocation or copy at every
  boundary.
- [ ] Generate cleanup on every partial-failure path. Use tests with forced
  allocator failures to prove previously initialized child values are freed.
- [ ] Add property tests for representative nested values and compare semantic
  equality after a complete Go/Rust round trip.
- [ ] Run `make wasm-api-generate`, the Go value tests, and
  `cargo test --manifest-path internal/wasm/runtime/native/Cargo.toml
  -p gate-wasm-native generated_values`.
- [ ] Commit as `feat: generate wasm value conversions`.

---

### Task 4: Generated Go operation dispatchers

**Files:**

- Create: `internal/wasm/generate/go_dispatch.go`
- Create: `internal/wasm/runtime/dispatch/host.go`
- Create: `internal/wasm/runtime/dispatch/errors.go`
- Generate: `internal/wasm/api/dispatch_gen.go`
- Create: `internal/wasm/api/dispatch_gen_test.go`

- [ ] Add failing generated fixture tests for package functions, constructors,
  value methods, resource methods, variable accessors, constants, variadics,
  options, multi-results, errors, and monomorphized generic operations.
- [ ] Generate a typed Go function for every callable manifest entry. Resolve
  resource handles, decode values, invoke the real Gate declaration, encode the
  result, and attach package-qualified operation identity to failures.
- [ ] Recover panics at every generated Go entry point. Return a stable host
  panic error and let the plugin failure policy decide whether execution may
  continue.
- [ ] Generate compile-time references to every represented declaration so a
  breaking rename or signature change fails Go compilation even before the
  artifact drift check.
- [ ] Assert generated dispatch operation count and IDs exactly match the
  manifest.
- [ ] Run `go test ./internal/wasm/api ./internal/wasm/runtime/dispatch -count=1`.
- [ ] Commit as `feat: generate Gate wasm dispatchers`.

---

### Task 5: Generated Rust Component Model host adapters

**Files:**

- Create: `internal/wasm/generate/rust_dispatch.go`
- Modify: `internal/wasm/runtime/native/host/src/engine.rs`
- Modify: `internal/wasm/runtime/native/host/src/lib.rs`
- Generate:
  `internal/wasm/runtime/native/host/src/generated/bindings.rs`
- Generate:
  `internal/wasm/runtime/native/host/src/generated/dispatch.rs`
- Generate: `internal/wasm/api/gate_wasm_generated.h`

- [ ] Add a failing component fixture importing representative operations from
  at least six Gate packages and exercising values, resources, and errors.
- [ ] Point Wasmtime bindgen at the production `gate.wit`. Generate host trait
  adapters that convert WIT values to C ABI calls and normalize returned
  status/error values.
- [ ] Replace the spike-specific sample callbacks and C declarations with the
  generated header and adapters. Retain the proven engine configuration,
  limits, panic containment, watchdog handling, and static version probe.
- [ ] Add a build-time assertion that generated WIT hash, Rust hash, C header
  hash, and Go dispatcher hash agree.
- [ ] Run `make wasm-native-test` and all Rust workspace tests.
- [ ] Commit as `feat: generate Wasmtime host adapters`.

---

### Task 6: Generated callbacks and synchronous re-entry

**Files:**

- Create: `internal/wasm/generate/callbacks.go`
- Create: `internal/wasm/runtime/callbacks/table.go`
- Create: `internal/wasm/runtime/callbacks/table_test.go`
- Generate: `internal/wasm/api/callbacks_gen.go`
- Generate:
  `internal/wasm/runtime/native/host/src/generated/callbacks.rs`

- [ ] Add failing tests for function-valued parameters, callback results,
  callback errors, command-style retained registrations, nested callbacks,
  guest traps, expired borrowed resources, and calling from the wrong thread.
- [ ] Generate guest callback identities and typed guest export entry points.
  Retained callbacks belong to the plugin until their registration is removed;
  borrowed callback resources expire at return.
- [ ] Route host-to-guest calls through the serialized executor, except nested
  same-plugin calls, which must use the proven scoped `ActiveCall` inline.
- [ ] Preserve exact call/return ordering and propagate nested trap, fuel,
  deadline, and host errors to the outer invocation.
- [ ] Repeat nested and cleanup tests 100 times with leak counters at baseline.
- [ ] Commit as `feat: generate typed wasm callbacks`.

---

### Task 7: Full coverage vertical test and spike removal

**Files:**

- Create: `internal/wasm/runtime/native/full_api_integration_test.go`
- Create: `internal/wasm/runtime/native/testdata/full-api-guest/...`
- Remove spike-only WIT/sample APIs after equivalent production tests exist
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`

- [ ] Generate a component fixture from production WIT that initializes with
  context and proxy, calls representative declarations across all lowering
  categories, registers a callback, and performs same-component re-entry.
- [ ] Add a manifest-driven test that invokes or compile-checks every generated
  operation and asserts no dispatcher ID is missing on either side.
- [ ] Remove the hand-written spike `Sample`, `Transform`, `Allocate`, and
  `Spin` surface only after their production equivalents cover records, lists,
  resources, host calls, limits, and traps.
- [ ] Make `wasm-native-test` regenerate/check the API before building Rust.
- [ ] Run full Go/Rust tests, Clippy, rustfmt, the 100-run cleanup suite, and the
  Linux/macOS/Windows native CI matrix.
- [ ] Commit as `feat: complete the generated Gate wasm bridge`.

---

## Completion checkpoint

Do not integrate plugin discovery into Gate startup until:

- manifest, WIT, C, Go, and Rust operation counts and hashes agree;
- all represented declarations compile against generated dispatchers;
- resource isolation and lifetime tests pass under `-race`;
- callback re-entry retains the feasibility spike's ordering and safety;
- no spike-specific host API remains.
