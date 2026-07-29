# WebAssembly Runtime Feasibility Results

Date: 2026-07-28

## Versions

- Go: `go1.26.2 darwin/arm64`
- Rust: `rustc 1.94.0 (4a4ef493e 2026-03-02)`
- Wasmtime: `47.0.2`
- wit-bindgen: `0.60.0`
- wit-component: `0.254.0`

## Result

PASS

## Verified behavior

| Behavior | Exact test or CI job | Observed result |
| --- | --- | --- |
| Go to Rust to component and back | `TestRuntime_NestedComponentCall` | Passed. Go constructed the Rust runtime, invoked component `init` and `on-event` exports, and received the expected values. |
| Records, lists, and results | `init_crosses_values_and_resources`; `TestRuntime_NestedComponentCall` | Passed. The `sample` record, its string list, and WIT results crossed the component, Rust, C, and Go boundaries without JSON. |
| Typed resources | `init_crosses_values_and_resources`; `TestRuntime_NestedComponentCall` | Passed. Borrowed context and proxy resources resolved to the expected host identities. |
| Guest-to-Go host calls | `TestRuntime_NestedComponentCall` | Passed. Component imports called Go `ContextCancelled`, `Transform`, and `EmitNested` implementations. |
| Nested same-component re-entry | `nested_event_reenters_same_component`; `TestRuntime_NestedComponentCall` | Passed with exact order `guest:outer`, `host:emit-nested`, `guest:inner`, `host:return-nested`; a retained Go re-entry token returned `ErrExpiredReentry`. |
| Trap propagation | `nested_guest_trap_traps_outer_invocation` | Passed. A trap in the nested component call trapped the outer invocation and preserved the expected call prefix. |
| Fuel interruption | `TestRuntime_LimitsFuel` | Passed. An infinite component loop returned `ErrFuelExhausted` within one second. |
| Deadline interruption | `TestRuntime_LimitsDeadline` | Passed. Epoch interruption returned `ErrDeadline` for a 25 ms deadline within one second. |
| Deterministic cleanup | `dropping_engine_releases_host_exactly_once`; `TestRuntime_CloseAfterFailedCall` | Passed. Rust dropped its host exactly once; Go deleted its cgo handle exactly once and made `Close` idempotent after failure. |
| Linux static linkage | `wasm-native-feasibility (ubuntu-latest, 1.94.0)` in CI run `30405666372` | Passed. `TestNativeLibraryIsLinked` returned `wasmtime-47.0.2` from the statically linked Rust archive. |
| macOS static linkage | `wasm-native-feasibility (macos-latest, 1.94.0)` in CI run `30405666372` | Passed. `TestNativeLibraryIsLinked` returned `wasmtime-47.0.2` from the statically linked Rust archive. |
| Windows static linkage | `wasm-native-feasibility (windows-latest, 1.94.0-x86_64-pc-windows-gnu)` in CI run `30405666372` | Passed. The GNU Rust archive linked into the MinGW CGO binary and `TestNativeLibraryIsLinked` returned `wasmtime-47.0.2`. |

The limit suite also passed memory growth and inbound/outbound transfer checks
in `TestRuntime_LimitsMemory` and `TestRuntime_LimitsTransfer`. The focused
nested-call, limit, and cleanup tests passed 100 consecutive executions.

## Sizes

Measured from a release build on Apple M4 Pro, macOS arm64:

| Artifact | Bytes |
| --- | ---: |
| `libgate_wasm_native.a` | 51,848,976 |
| `gate_wasm_spike.component.wasm` | 33,974 |

The archive is an intermediate static library. Final Gate binary-size impact
will be smaller than the archive size after the platform linker removes
unreferenced objects, and must be measured again in the product release plan.

## Benchmarks

Measured on Apple M4 Pro, macOS arm64, with release Rust artifacts. Go
benchmarks used a one-second benchtime and three runs; the table reports the
median. Rust compilation and instantiation measurements used 20 and 100
iterations respectively.

| Operation | Time | Go allocations |
| --- | ---: | ---: |
| Cold component compilation, Rust measurement | 22.631 ms | n/a |
| Component instantiation only, Rust measurement | 16.605 µs | n/a |
| Cold compile and instantiate through Go | 22.447 ms | 4 allocs, 120 B |
| Scalar component call | 16.606 µs | 2 allocs, 32 B |
| Record/list component call | 19.909 µs | 16 allocs, 248 B |
| Nested guest-to-Go-to-same-guest callback | 22.786 µs | 15 allocs, 272 B |

## Safety review

- **Re-entry lifetime:** Rust's `ActiveCall<'a>` owns Wasmtime's scoped
  `Access<'a, ...>` and cannot outlive the active host import. Its compile-fail
  documentation test proves it cannot be widened to `'static`. The Go
  `Reentry` wrapper is explicitly expired when the host callback returns, and
  later calls fail with `ErrExpiredReentry`.
- **Thread affinity:** `ActiveCall` is `!Send` and `!Sync` through
  `PhantomData<Rc<()>>`. The Go callback locks its goroutine to the current OS
  thread and validates the native thread identity before re-entering. Each
  runtime serializes top-level calls with a mutex. Wasmtime concurrency support
  is disabled because the synchronous component access API is the mode that
  permits supported nested re-entry.
- **Pointer ownership:** Rust retains only the integer value of a
  `runtime/cgo.Handle`, never a Go pointer. Go input buffers are borrowed only
  for a call and protected with `runtime.KeepAlive`. Rust and C output buffers
  have explicit owner-specific free functions and are copied before release.
- **Panic containment:** Every exported Rust C entry point catches Rust panics
  before returning through C. Every exported Go callback recovers Go panics and
  returns an owned error buffer. Neither language unwinds across the FFI
  boundary.
- **Cleanup:** Runtime close drops the Wasmtime store before deleting the Go
  cgo handle, is idempotent, and remains valid after a failed or interrupted
  call. Deadline watchdogs are canceled and joined before the call returns.
  Repeated tests leave both Rust host drops and Go live-handle counts at their
  starting values.

## Decision

Proceed with the canonical analyzer, generated full-API bridge, product
runtime, and language-neutral example plans. The mandatory runtime gate passed:
the supported Wasmtime API can provide typed Component Model calls, static
Go/Rust linkage on all Gate CI platforms, synchronous same-component re-entry,
bounded execution, and deterministic cleanup without weakening the approved
plugin behavior.
