# WebAssembly Runtime Feasibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove that Gate can synchronously call a WebAssembly Component
through a statically linked Rust Wasmtime host, expose typed host resources,
cross the Go/Rust boundary, re-enter the same component during a nested Gate
callback, interrupt runaway code, and clean up deterministically.

**Architecture:** A Rust workspace builds a minimal WIT-based guest component
and a `staticlib` Wasmtime host. An opt-in CGO Go package links the library and
provides a typed `Runtime` API. A scoped re-entry token lets a Go host callback
synchronously invoke a guest export through the active Wasmtime
`StoreContextMut`; the token is thread-bound and invalid after its host call.
This plan is the mandatory feasibility gate from the approved design. Analyzer
and full-API plans are written only after its outcome is known.

**Tech Stack:** Go 1.26, CGO, Rust 1.94.0, Wasmtime 47.0.2 Component Model,
WIT, wit-bindgen 0.60.0, wit-component 0.254.0, wasm32-unknown-unknown.

## Global Constraints

- Work only in `/Users/robin/.treehouse/gate-68e4cf/3/gate` on
  `feat/wasm-plugin-support`.
- Keep the default `go test ./...` path usable before the production release
  build is converted from `CGO_ENABLED=0`; native tests use the
  `wasm_native` build tag.
- The host runtime is a statically linked Rust library, not a sidecar.
- Components use WIT and the Component Model, not `syscall/js`.
- Cross-boundary calls use typed C-compatible values, buffers, and resource
  IDs, never JSON RPC.
- Rust and Go must never retain a Go pointer across the FFI boundary.
- Guest memory default: 128 MiB.
- One-call transfer default: 16 MiB.
- Live-handle default: 65,536.
- Initialization deadline: 10 seconds.
- Callback deadline: 100 milliseconds.
- Wasmtime fuel is enabled.
- No WASI interfaces are linked into the spike component.
- Nested same-component re-entry is required. If it cannot be implemented
  safely with supported Wasmtime APIs, stop after documenting the exact
  blocker; do not weaken the behavior by queueing the callback.
- Every task follows red-green-refactor and ends in its own commit.

---

## Planned File Structure

```text
rust-toolchain.toml
Makefile
.github/workflows/ci.yml
internal/wasm/runtime/native/
  Cargo.toml                         Rust workspace and pinned dependencies
  Cargo.lock                        reproducible native and guest dependency graph
  wit/spike.wit                     minimal component contract
  guest-spike/
    Cargo.toml                       wasm32 guest fixture crate
    src/lib.rs                       fixture behavior for init/event/spin
  componentize/
    Cargo.toml                       core-Wasm-to-component build tool
    src/lib.rs                       reusable component encoder
    src/main.rs                      deterministic CLI wrapper
  host/
    Cargo.toml                       Wasmtime static library crate
    include/gate_wasm_native.h       stable spike C ABI
    src/abi.rs                       C values, buffers, status, ownership
    src/engine.rs                    engine, component, store, instance
    src/reentry.rs                   scoped active-call re-entry token
    src/lib.rs                       exported C functions and panic containment
    tests/component.rs               Rust-only vertical runtime tests
  artifacts/.gitkeep                output directory; generated wasm is ignored
  bridge.go                          public Go wrapper, wasm_native+cgo
  callbacks.go                       Go host callbacks exported to C
  bridge.c                           C function-pointer invocation shims
  bridge.h                           declarations shared with cgo
  disabled.go                        default-build unavailable implementation
  runtime.go                         build-independent types and errors
  runtime_test.go                    default-build behavior
  integration_test.go               Go/Rust/component tests, wasm_native+cgo
  testdata/.gitkeep                  stable test-data directory
docs/superpowers/specs/
  2026-07-28-wasm-runtime-feasibility-results.md
```

The generated component and native libraries stay under ignored `target/` or
`artifacts/` directories. Source, WIT, lockfiles, tests, and headers are
committed.

---

### Task 1: Reproducible WIT Guest Component

**Files:**

- Create: `rust-toolchain.toml`
- Modify: `.gitignore`
- Modify: `Makefile`
- Create: `internal/wasm/runtime/native/Cargo.toml`
- Create: `internal/wasm/runtime/native/wit/spike.wit`
- Create: `internal/wasm/runtime/native/guest-spike/Cargo.toml`
- Create: `internal/wasm/runtime/native/guest-spike/src/lib.rs`
- Create: `internal/wasm/runtime/native/componentize/Cargo.toml`
- Create: `internal/wasm/runtime/native/componentize/src/lib.rs`
- Create: `internal/wasm/runtime/native/componentize/src/main.rs`
- Create: `internal/wasm/runtime/native/artifacts/.gitkeep`

**Interfaces:**

- Produces:
  `internal/wasm/runtime/native/artifacts/gate_wasm_spike.component.wasm`.
- Produces Rust function
  `componentize::encode(module: &[u8]) -> anyhow::Result<Vec<u8>>`.
- Component imports `minekube:gate-spike/host@0.1.0`.
- Component exports `minekube:gate-spike/plugin@0.1.0`.

- [ ] **Step 1: Add the contract and failing componentizer test**

Create this WIT contract:

```wit
package minekube:gate-spike@0.1.0;

interface host {
  record sample {
    text: string,
    factor: s32,
    tags: list<string>,
  }

  resource context {
    is-cancelled: func() -> bool;
  }

  resource proxy {
    transform: func(input: sample) -> result<sample, string>;
    emit-nested: func(input: string) -> result<string, string>;
  }
}

interface plugin {
  use host.{context, proxy, sample};

  init: func(
    context: borrow<context>,
    proxy: borrow<proxy>,
  ) -> result<sample, string>;

  on-event: func(
    proxy: borrow<proxy>,
    input: string,
  ) -> result<string, string>;

  allocate: func(bytes: u64) -> u64;

  spin: func();
}

world gate-plugin {
  import host;
  export plugin;
}
```

Add a unit test in `componentize/src/lib.rs` that calls
`encode(b"not wasm")` and asserts the error includes `failed to decode core
module`. Declare `encode` but initially return `unimplemented!()`.

- [ ] **Step 2: Run the test and verify red**

Run:

```bash
cargo test --manifest-path internal/wasm/runtime/native/Cargo.toml \
  -p gate-wasm-componentize
```

Expected: FAIL because `encode` is not implemented.

- [ ] **Step 3: Implement deterministic component encoding**

Implement `encode` with `wit_component::ComponentEncoder`:

```rust
pub fn encode(module: &[u8]) -> anyhow::Result<Vec<u8>> {
    wit_component::ComponentEncoder::default()
        .module(module)
        .context("failed to decode core module")?
        .validate(true)
        .encode()
        .context("failed to encode component")
}
```

The CLI accepts exactly two positional paths, reads the core module, calls
`encode`, creates the output parent, and writes the component. Invalid argument
count exits with:

```text
usage: gate-wasm-componentize <core-module.wasm> <component.wasm>
```

- [ ] **Step 4: Implement the guest fixture**

Use `wit_bindgen::generate!` against `../wit`, implement the generated guest
traits, and export the component:

```rust
wit_bindgen::generate!({
    path: "../wit",
    world: "gate-plugin",
});

struct Spike;

impl exports::minekube::gate_spike::plugin::Guest for Spike {
    fn init(
        context: minekube::gate_spike::host::ContextBorrow<'_>,
        proxy: minekube::gate_spike::host::ProxyBorrow<'_>,
    ) -> Result<minekube::gate_spike::host::Sample, String> {
        if context.is_cancelled() {
            return Err("context cancelled".into());
        }
        proxy.transform(&minekube::gate_spike::host::Sample {
            text: "init".into(),
            factor: 2,
            tags: vec!["guest".into(), "component".into()],
        })
    }

    fn on_event(
        proxy: minekube::gate_spike::host::ProxyBorrow<'_>,
        input: String,
    ) -> Result<String, String> {
        if input == "outer" {
            return Ok(format!("outer:{}", proxy.emit_nested("inner")?));
        }
        Ok(format!("guest:{input}"))
    }

    fn allocate(bytes: u64) -> u64 {
        let allocation = vec![0_u8; usize::try_from(bytes).unwrap()];
        std::hint::black_box(&allocation);
        allocation.len() as u64
    }

    fn spin() {
        loop {
            core::hint::spin_loop();
        }
    }
}

export!(Spike);
```

If wit-bindgen 0.60.0 generates owned resource wrapper names instead of the
shown borrow aliases, use the exact generated 0.60.0 signatures while
preserving the WIT contract and behavior.

- [ ] **Step 5: Add the reproducible build target**

Add Make targets:

```make
WASM_NATIVE_DIR := internal/wasm/runtime/native
WASM_SPIKE_CORE := $(WASM_NATIVE_DIR)/target/wasm32-unknown-unknown/release/gate_wasm_spike_guest.wasm
WASM_SPIKE_COMPONENT := $(WASM_NATIVE_DIR)/artifacts/gate_wasm_spike.component.wasm

wasm-spike-component:
	cd $(WASM_NATIVE_DIR) && cargo build -p gate-wasm-spike-guest --release --target wasm32-unknown-unknown
	cd $(WASM_NATIVE_DIR) && cargo run -p gate-wasm-componentize --release -- \
		target/wasm32-unknown-unknown/release/gate_wasm_spike_guest.wasm \
		artifacts/gate_wasm_spike.component.wasm
```

Ignore `internal/wasm/runtime/native/target/` and
`internal/wasm/runtime/native/artifacts/*.wasm`, retaining `.gitkeep`.

- [ ] **Step 6: Build and validate the component**

Run:

```bash
make wasm-spike-component
cargo test --manifest-path internal/wasm/runtime/native/Cargo.toml \
  -p gate-wasm-componentize
```

Expected: both commands exit 0, and the component file starts with the
WebAssembly magic bytes `00 61 73 6d`.

- [ ] **Step 7: Commit**

```bash
git add rust-toolchain.toml .gitignore Makefile \
  internal/wasm/runtime/native
git commit -m "build: add wasm component spike fixture"
```

---

### Task 2: Rust Wasmtime Host With Values and Resources

**Files:**

- Create: `internal/wasm/runtime/native/host/Cargo.toml`
- Create: `internal/wasm/runtime/native/host/src/abi.rs`
- Create: `internal/wasm/runtime/native/host/src/engine.rs`
- Create: `internal/wasm/runtime/native/host/src/reentry.rs`
- Create: `internal/wasm/runtime/native/host/src/lib.rs`
- Create: `internal/wasm/runtime/native/host/include/gate_wasm_native.h`
- Create: `internal/wasm/runtime/native/host/tests/component.rs`
- Modify: `internal/wasm/runtime/native/Cargo.toml`

**Interfaces:**

- Produces Rust
  `Engine::new(component: &[u8], host: Arc<dyn Host>, limits: Limits)`.
- Produces
  `Engine::init(context: u64, proxy: u64) -> anyhow::Result<Sample>`.
- Produces
  `Engine::on_event(proxy: u64, input: &str) -> anyhow::Result<String>`.
- Produces `Engine::allocate(bytes: u64) -> anyhow::Result<u64>` and
  `Engine::spin() -> anyhow::Result<()>`.
- Produces typed resource IDs for the borrowed context and proxy.
- Consumes the component produced by Task 1.

- [ ] **Step 1: Write the failing Rust vertical-call test**

Define:

```rust
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Sample {
    pub text: String,
    pub factor: i32,
    pub tags: Vec<String>,
}

pub trait Host: Send + Sync + 'static {
    fn context_is_cancelled(&self, context: u64) -> anyhow::Result<bool>;
    fn proxy_transform(&self, proxy: u64, input: Sample)
        -> anyhow::Result<Result<Sample, String>>;
    fn proxy_emit_nested(
        &self,
        active: &mut ActiveCall<'_>,
        proxy: u64,
        input: String,
    ) -> anyhow::Result<Result<String, String>>;
}
```

The test host asserts context ID `1`, proxy ID `2`, transforms `text` to
`"host:init"`, multiplies `factor` by `3`, and appends `"host"` to `tags`.
The test loads the Task 1 component and expects:

```rust
Sample {
    text: "host:init".into(),
    factor: 6,
    tags: vec!["guest".into(), "component".into(), "host".into()],
}
```

- [ ] **Step 2: Run the test and verify red**

Run:

```bash
make wasm-spike-component
cargo test --manifest-path internal/wasm/runtime/native/Cargo.toml \
  -p gate-wasm-native --test component init_crosses_values_and_resources
```

Expected: FAIL because the host engine and imported resource bindings do not
exist.

- [ ] **Step 3: Implement the minimum Wasmtime engine**

Configure:

```rust
let mut config = wasmtime::Config::new();
config.wasm_component_model(true);
config.consume_fuel(true);
config.epoch_interruption(true);
let engine = wasmtime::Engine::new(&config)?;
```

Use `wasmtime::component::bindgen!` for the spike world. Store data contains:

```rust
struct StoreData {
    host: Arc<dyn Host>,
    table: wasmtime::component::ResourceTable,
    instance: Option<wasmtime::component::Instance>,
    context_id: u64,
    proxy_id: u64,
}
```

Do not add WASI to the linker. Implement imported context and proxy resources,
instantiate the component, save its `Instance`, and call the generated `init`
export with borrowed resources backed by IDs `1` and `2`.

- [ ] **Step 4: Run values/resource tests**

Run:

```bash
cargo test --manifest-path internal/wasm/runtime/native/Cargo.toml \
  -p gate-wasm-native --test component init_crosses_values_and_resources
```

Expected: PASS and no unlinked WASI imports.

- [ ] **Step 5: Add ownership regression tests**

Add tests proving:

- the guest cannot drop borrowed context or proxy roots;
- a proxy resource cannot be used with the context resource type;
- dropping the `Engine` drops its `StoreData`, resource table, and host
  `Arc` exactly once.

Use an `Arc<AtomicUsize>` drop guard and assert the count is `1`.

- [ ] **Step 6: Run the host test suite**

Run:

```bash
cargo test --manifest-path internal/wasm/runtime/native/Cargo.toml \
  -p gate-wasm-native
```

Expected: all host tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/wasm/runtime/native
git commit -m "feat: host wasm components with typed resources"
```

---

### Task 3: Same-Component Synchronous Re-entry

**Files:**

- Modify: `internal/wasm/runtime/native/host/src/engine.rs`
- Modify: `internal/wasm/runtime/native/host/src/reentry.rs`
- Modify: `internal/wasm/runtime/native/host/tests/component.rs`

**Interfaces:**

- Produces `ActiveCall::on_event(proxy: u64, input: &str)`.
- `ActiveCall` is valid only during the corresponding host import.
- Uses Wasmtime's supported component `StoreContextMut` re-entry path.

- [ ] **Step 1: Write the nested re-entry test**

The host implementation of `proxy_emit_nested` calls:

```rust
active.on_event(proxy, &input)
```

Call:

```rust
let output = runtime.on_event(2, "outer")?;
assert_eq!(output, "outer:guest:inner");
```

Also record call order and assert:

```rust
assert_eq!(
    calls,
    ["guest:outer", "host:emit-nested", "guest:inner", "host:return-nested"]
);
```

- [ ] **Step 2: Run the test and verify red**

Run:

```bash
cargo test --manifest-path internal/wasm/runtime/native/Cargo.toml \
  -p gate-wasm-native --test component nested_event_reenters_same_component
```

Expected: FAIL because `ActiveCall::on_event` is not implemented.

- [ ] **Step 3: Implement scoped re-entry**

Register `emit-nested` with the dynamic Component Model linker API so its host
closure receives `StoreContextMut`. Construct `ActiveCall` as a non-`Send`,
non-`Sync` scoped wrapper around that context plus the copied component
`Instance`. Re-enter the generated `on-event` export through this same context.

Enforce:

```rust
pub struct ActiveCall<'a> {
    store: wasmtime::StoreContextMut<'a, StoreData>,
    instance: wasmtime::component::Instance,
    _thread_bound: PhantomData<Rc<()>>,
}
```

No `StoreContextMut`, reference, or raw pointer may escape the host import
closure. Borrowed resources passed to the nested call are released before
returning to the outer guest call.

- [ ] **Step 4: Test nested success, nested trap, and expired use**

Add:

- nested success returns `outer:guest:inner`;
- nested guest trap propagates to and traps the outer invocation;
- an attempted `ActiveCall` use after the import returns is rejected by the
  Rust type system, documented with a `compile_fail` doctest.

Run:

```bash
cargo test --manifest-path internal/wasm/runtime/native/Cargo.toml \
  -p gate-wasm-native
```

Expected: all tests and the compile-fail doctest pass.

- [ ] **Step 5: Apply the feasibility stop condition**

If Wasmtime returns `cannot enter component`, cannot represent the active
borrow safely, or requires unsupported internal APIs, do not add a queue.
Instead, skip Tasks 4-6 and proceed directly to Task 7 with a failed result
that includes the smallest reproducer and Wasmtime diagnostic.

- [ ] **Step 6: Commit**

```bash
git add internal/wasm/runtime/native/host
git commit -m "feat: prove synchronous wasm component reentry"
```

---

### Task 4: Static Rust-to-Go Bridge

**Files:**

- Create: `internal/wasm/runtime/native/runtime.go`
- Create: `internal/wasm/runtime/native/runtime_test.go`
- Create: `internal/wasm/runtime/native/disabled.go`
- Create: `internal/wasm/runtime/native/bridge.go`
- Create: `internal/wasm/runtime/native/callbacks.go`
- Create: `internal/wasm/runtime/native/bridge.c`
- Create: `internal/wasm/runtime/native/bridge.h`
- Create: `internal/wasm/runtime/native/integration_test.go`
- Modify: `internal/wasm/runtime/native/host/src/abi.rs`
- Modify: `internal/wasm/runtime/native/host/src/lib.rs`
- Modify: `internal/wasm/runtime/native/host/include/gate_wasm_native.h`
- Modify: `Makefile`

**Interfaces:**

- Produces Go `New(component []byte, host Host, limits Limits)`.
- Produces Go
  `Runtime.Init(contextID, proxyID uint64) (Sample, error)`.
- Produces Go
  `Runtime.OnEvent(proxyID uint64, input string) (string, error)`.
- Produces Go `Runtime.Allocate(bytes uint64) (uint64, error)`,
  `Runtime.Spin() error`, and `Runtime.Close() error`.
- Produces Go `Reentry.OnEvent`, valid only during `Host.EmitNested`.

- [ ] **Step 1: Add build-independent Go types and red default-build test**

Define:

```go
var ErrUnavailable = errors.New("wasm native runtime unavailable")
var ErrClosed = errors.New("wasm runtime closed")
var ErrExpiredReentry = errors.New("wasm reentry token expired")

type Sample struct {
	Text   string
	Factor int32
	Tags   []string
}

type Limits struct {
	MemoryBytes uint64
	TransferBytes uint64
	Fuel          uint64
	Deadline      time.Duration
}

type Host interface {
	ContextCancelled(contextID uint64) (bool, error)
	Transform(proxyID uint64, input Sample) (Sample, error)
	EmitNested(reentry Reentry, proxyID uint64, input string) (string, error)
}

type Reentry interface {
	OnEvent(proxyID uint64, input string) (string, error)
}
```

With no build tags, `New` returns `ErrUnavailable`. Test this with:

```go
func TestDisabledRuntime(t *testing.T) {
	_, err := New(nil, nil, Limits{})
	require.ErrorIs(t, err, ErrUnavailable)
}
```

- [ ] **Step 2: Verify the default path**

Run:

```bash
go test ./internal/wasm/runtime/native
go test ./...
```

Expected: both pass without invoking Cargo or requiring CGO.

- [ ] **Step 3: Define the C ABI**

The committed header defines fixed-width types:

```c
typedef struct gate_wasm_slice {
  const uint8_t *ptr;
  size_t len;
} gate_wasm_slice;

typedef struct gate_wasm_owned_bytes {
  uint8_t *ptr;
  size_t len;
  size_t cap;
} gate_wasm_owned_bytes;

typedef struct gate_wasm_slice_list {
  const gate_wasm_slice *ptr;
  size_t len;
} gate_wasm_slice_list;

typedef struct gate_wasm_owned_bytes_list {
  gate_wasm_owned_bytes *ptr;
  size_t len;
  size_t cap;
} gate_wasm_owned_bytes_list;

typedef struct gate_wasm_sample_view {
  gate_wasm_slice text;
  int32_t factor;
  gate_wasm_slice_list tags;
} gate_wasm_sample_view;

typedef struct gate_wasm_owned_sample {
  gate_wasm_owned_bytes text;
  int32_t factor;
  gate_wasm_owned_bytes_list tags;
} gate_wasm_owned_sample;

typedef struct gate_wasm_limits {
  uint64_t memory_bytes;
  uint64_t transfer_bytes;
  uint64_t fuel;
  uint64_t deadline_nanos;
} gate_wasm_limits;

typedef struct gate_wasm_runtime gate_wasm_runtime;
typedef struct gate_wasm_reentry gate_wasm_reentry;

gate_wasm_runtime *gate_wasm_runtime_new(
    gate_wasm_slice component,
    uintptr_t go_host,
    gate_wasm_limits limits,
    gate_wasm_owned_bytes *error);
int32_t gate_wasm_runtime_init(
    gate_wasm_runtime *, uint64_t context_id, uint64_t proxy_id,
    gate_wasm_owned_sample *output,
    gate_wasm_owned_bytes *error);
int32_t gate_wasm_runtime_on_event(
    gate_wasm_runtime *, uint64_t proxy_id, gate_wasm_slice input,
    gate_wasm_owned_bytes *output, gate_wasm_owned_bytes *error);
int32_t gate_wasm_runtime_allocate(
    gate_wasm_runtime *, uint64_t bytes, uint64_t *output,
    gate_wasm_owned_bytes *error);
int32_t gate_wasm_runtime_spin(
    gate_wasm_runtime *, gate_wasm_owned_bytes *error);
void gate_wasm_runtime_free(gate_wasm_runtime *);
void gate_wasm_owned_bytes_free(gate_wasm_owned_bytes);
void gate_wasm_owned_sample_free(gate_wasm_owned_sample);
```

Rust owns Rust output buffers; Go copies them before calling
`gate_wasm_owned_bytes_free` or `gate_wasm_owned_sample_free`. Go callback
strings and samples are allocated with `C.malloc`; Rust copies them and calls
`C.free`. Neither side retains input pointers.

`bridge.h` also declares the Go callback ABI used by Rust:

```c
int32_t gate_wasm_go_context_cancelled(
    uintptr_t host, uint64_t context_id, uint8_t *cancelled,
    gate_wasm_owned_bytes *error);
int32_t gate_wasm_go_transform(
    uintptr_t host, uint64_t proxy_id, gate_wasm_sample_view input,
    gate_wasm_owned_sample *output, gate_wasm_owned_bytes *error);
int32_t gate_wasm_go_emit_nested(
    uintptr_t host, gate_wasm_reentry *reentry, uint64_t proxy_id,
    gate_wasm_slice input, gate_wasm_owned_bytes *output,
    gate_wasm_owned_bytes *error);
```

- [ ] **Step 4: Export panic-safe Rust functions**

Every exported Rust C function wraps its body in `catch_unwind`, maps success
to status `0`, typed errors to nonzero status, and never unwinds through C.
`gate_wasm_runtime_free(NULL)` is a no-op. Repeated Go `Close` calls are safe.

- [ ] **Step 5: Implement Go callbacks and the re-entry trampoline**

Use `runtime/cgo.Handle` for host identity; pass only its integer value to
Rust. `bridge.c` calls the `//export` Go callbacks and invokes the Rust re-entry
function pointer because Go cannot directly call a C function pointer.

During `EmitNested`, Go receives a scoped C re-entry token and wraps it in a Go
value implementing `Reentry`. The wrapper:

- checks an atomic active flag;
- requires the callback goroutine to remain on its locked OS thread;
- calls the C trampoline synchronously;
- becomes expired before the Go callback returns;
- returns `ErrExpiredReentry` on later use.

- [ ] **Step 6: Write and run the cross-language nested test**

The Go test host transforms the init sample and implements:

```go
func (h *testHost) EmitNested(
	reentry Reentry,
	proxyID uint64,
	input string,
) (string, error) {
	h.calls = append(h.calls, "host:emit-nested")
	result, err := reentry.OnEvent(proxyID, input)
	h.calls = append(h.calls, "host:return-nested")
	return result, err
}
```

Assert `Init(1, 2)` returns the transformed record and
`OnEvent(2, "outer")` returns `outer:guest:inner`. Retain the `Reentry` value
and assert it returns `ErrExpiredReentry` after the outer call finishes.

Run:

```bash
make wasm-native-test
```

The Make target builds the component and Rust `staticlib`, then runs:

```bash
CGO_ENABLED=1 go test -tags=wasm_native ./internal/wasm/runtime/native
```

Expected: PASS with the exact nested output and call order.

- [ ] **Step 7: Verify default and native suites**

Run:

```bash
go test ./...
make wasm-native-test
cargo clippy --manifest-path internal/wasm/runtime/native/Cargo.toml \
  --workspace --all-targets -- -D warnings
cargo fmt --manifest-path internal/wasm/runtime/native/Cargo.toml --all -- --check
```

Expected: all commands pass.

- [ ] **Step 8: Commit**

```bash
git add Makefile internal/wasm/runtime/native
git commit -m "feat: bridge Gate Go to Wasmtime components"
```

---

### Task 5: Interruption, Limits, and Cleanup

**Files:**

- Modify: `internal/wasm/runtime/native/host/src/engine.rs`
- Modify: `internal/wasm/runtime/native/host/src/lib.rs`
- Modify: `internal/wasm/runtime/native/host/tests/component.rs`
- Modify: `internal/wasm/runtime/native/bridge.go`
- Modify: `internal/wasm/runtime/native/integration_test.go`

**Interfaces:**

- Consumes `Limits` from Task 4.
- Produces stable errors `ErrFuelExhausted`, `ErrDeadline`,
  `ErrMemoryLimit`, and `ErrTransferLimit`.

- [ ] **Step 1: Add failing limit tests**

Add tests proving:

- `Spin` terminates with `ErrFuelExhausted` when fuel is tiny;
- `Spin` terminates with `ErrDeadline` within one second when the deadline is
  25 milliseconds and fuel is high;
- `Allocate` cannot grow guest memory past `MemoryBytes`;
- a result above `TransferBytes` is rejected before copying to Go;
- closing during or after a failed call drops the store and host handle once.

- [ ] **Step 2: Run tests and verify red**

Run:

```bash
make wasm-native-test
```

Expected: FAIL because limits and interruption errors are not implemented.

- [ ] **Step 3: Implement store limits, fuel, and deadlines**

Use Wasmtime `StoreLimitsBuilder` for linear memory, `Store::set_fuel` before
each call, and epoch interruption for wall-clock deadlines. A watchdog thread
may increment the engine epoch, but it must be canceled and joined before the
call returns. Normalize traps by inspecting Wasmtime trap codes; do not match
only human-readable strings.

Enforce `TransferBytes` on both incoming and outgoing buffers before
allocation/copy. Zero values in `Limits` select the design defaults.

- [ ] **Step 4: Run focused and complete tests**

Run:

```bash
make wasm-native-test
go test ./...
```

Expected: all limit, cleanup, and existing Gate tests pass.

- [ ] **Step 5: Run leak-oriented repetitions**

Run:

```bash
CGO_ENABLED=1 go test -tags=wasm_native \
  ./internal/wasm/runtime/native -run 'TestRuntime_(Nested|Close|Limits)' \
  -count=100
```

Expected: PASS without growing live Rust runtime or Go cgo-handle counters.

- [ ] **Step 6: Commit**

```bash
git add internal/wasm/runtime/native
git commit -m "feat: enforce wasm runtime limits"
```

---

### Task 6: Supported-Platform Build Probe

**Files:**

- Modify: `.github/workflows/ci.yml`
- Modify: `Makefile`
- Create: `internal/wasm/runtime/native/README.md`

**Interfaces:**

- Produces CI evidence for Linux, macOS, and Windows native linkage.
- Does not yet change `.goreleaser.yml`; that belongs to the product-runtime
  plan after feasibility is accepted.

- [ ] **Step 1: Add a CI matrix that is initially red**

Add `wasm-native-feasibility` with:

```yaml
strategy:
  fail-fast: false
  matrix:
    platform: [ubuntu-latest, macos-latest, windows-latest]
runs-on: ${{ matrix.platform }}
```

Install Go from `go.mod`, Rust from `rust-toolchain.toml`, and run the
platform-neutral Make target `wasm-native-test`. The target must use
PowerShell-compatible Go/Cargo commands on Windows; isolate shell-specific
filesystem operations in Go or Rust tools.

- [ ] **Step 2: Add a static-link assertion**

Add `TestNativeLibraryIsLinked` under the `wasm_native && cgo` tags. It calls a
Rust-exported `gate_wasm_runtime_version` and expects `wasmtime-47.0.2`.
This ensures the Go test binary did not accidentally use a sidecar process.

- [ ] **Step 3: Document exact developer prerequisites**

The README states:

- Rust is selected by `rust-toolchain.toml`;
- no global `cargo-component`, `wasm-tools`, or `wit-bindgen` CLI is required;
- a platform C toolchain is required for CGO;
- `make wasm-native-test` is the complete local spike check;
- normal `go test ./...` remains available without Rust;
- release conversion from `CGO_ENABLED=0` is intentionally deferred until the
  feasibility result is accepted.

- [ ] **Step 4: Run local verification**

Run:

```bash
make wasm-native-test
go test ./...
git diff --check
```

Expected: all commands pass locally. CI matrix results are required before the
feasibility result can be marked cross-platform successful.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml Makefile \
  internal/wasm/runtime/native/README.md \
  internal/wasm/runtime/native
git commit -m "ci: verify native wasm runtime linkage"
```

---

### Task 7: Feasibility Result and Architecture Decision

**Files:**

- Create:
  `docs/superpowers/specs/2026-07-28-wasm-runtime-feasibility-results.md`
- Modify:
  `docs/superpowers/specs/2026-07-28-wasm-plugin-support-design.md`

**Interfaces:**

- Produces the explicit go/no-go input for the analyzer and full runtime plans.

- [ ] **Step 1: Record measured evidence**

Write the result document only after collecting evidence. It must contain the
title `WebAssembly Runtime Feasibility Results`, the date, and exact Go, Rust,
Wasmtime, wit-bindgen, and wit-component versions.

Add a `Result` section containing exactly `PASS` or `BLOCKED`. Add a verified
behavior table with one row for each of:

- Go to Rust to component and back;
- records, lists, and results;
- typed resources;
- guest-to-Go host calls;
- nested same-component re-entry;
- trap propagation;
- fuel interruption;
- deadline interruption;
- deterministic cleanup;
- Linux static linkage;
- macOS static linkage;
- Windows static linkage.

Each row contains the exact test or CI job name and its observed result. Add
measured native-library and component byte sizes plus benchmark results for
cold compilation, instantiation, a scalar call, a record/list call, and a
nested callback. Add a safety review covering re-entry lifetime, thread
affinity, pointer ownership, panic containment, and cleanup. Finish with the
decision to proceed to analyzer/full-API planning or the exact blocking
runtime limitation. Do not create the file until every required value is
known.

- [ ] **Step 2: Update design status**

On success, append a design note linking the result and stating that the
runtime feasibility gate passed. On failure, change the design status to
`Blocked by runtime feasibility` and link the reproducer and diagnostic.

- [ ] **Step 3: Run final verification**

Run:

```bash
go test ./...
make wasm-native-test
cargo test --manifest-path internal/wasm/runtime/native/Cargo.toml --workspace
cargo clippy --manifest-path internal/wasm/runtime/native/Cargo.toml \
  --workspace --all-targets -- -D warnings
cargo fmt --manifest-path internal/wasm/runtime/native/Cargo.toml --all -- --check
git diff --check
git status --short
```

Expected on success: all commands exit 0 and only the result/design
documentation is uncommitted. On a blocked result, run every command that does
not depend on the failed behavior and record the failures verbatim.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs
git commit -m "docs: record wasm runtime feasibility"
```

- [ ] **Step 5: Gate the next plans**

Only a `PASS` result authorizes writing:

1. the canonical Go API analyzer and WIT generator plan;
2. the generated Go/Rust dispatcher plan;
3. the plugin loader, lifecycle, events, timers, configuration, and release
   plan;
4. the language-neutral examples and documentation plan.
