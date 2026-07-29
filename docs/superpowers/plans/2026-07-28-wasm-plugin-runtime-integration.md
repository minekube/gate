# Gate WebAssembly Plugin Runtime Integration Plan

> **Execution requirement:** Begin after generated dispatcher completion. Use
> executing-plans and test-driven-development, with one focused commit per task.

**Goal:** Load `.wasm` components from configuration, initialize them with the
real Gate context and proxy before accepting connections, provide events,
commands, and timers, isolate failures, clean up deterministically, and ship
the statically linked runtime in supported release artifacts.

**Architecture:** A Go plugin manager owns discovery, compatibility validation,
per-plugin stores, executors, resource tables, registrations, and timers. It is
created with the Java proxy and invoked from the existing `initPlugins` point,
after proxy initialization and before `listenAndServe`. Generated dispatchers
provide the public API. The Rust library continues to own Wasmtime mechanics.

---

### Task 1: Configuration and validation

**Files:**

- Create: `pkg/edition/java/config/wasm.go`
- Create: `pkg/edition/java/config/wasm_test.go`
- Modify: `pkg/edition/java/config/config.go`
- Modify: `config.yml`
- Modify: `config-simple.yml`
- Modify: `config-lite.yml`
- Modify: `pkg/configs/config.yml`
- Modify: `pkg/configs/config-simple.yml`
- Modify: `pkg/configs/config-lite.yml`

- [ ] Add failing YAML/default/validation tests for global enablement,
  `plugins/` directory default, per-plugin enablement, memory, transfer, handle,
  timer, fuel, initialization deadline, and callback deadline overrides.
- [ ] Reject negative, overflowing, zero-when-not-defaultable, and
  host-dangerous values. Warn on unknown per-plugin identities after discovery,
  rather than silently ignoring likely misspellings.
- [ ] Keep Wasm plugins disabled by default for the first release unless the
  release task explicitly changes that after startup coverage is green.
- [ ] Sync embedded configs with `make sync-configs` and run config tests.
- [ ] Commit as `feat: configure wasm plugins`.

---

### Task 2: Deterministic discovery, metadata, and compatibility

**Files:**

- Create: `internal/wasm/runtime/plugin/metadata.go`
- Create: `internal/wasm/runtime/plugin/discover.go`
- Create: `internal/wasm/runtime/plugin/compatibility.go`
- Create: `internal/wasm/runtime/plugin/discover_test.go`
- Add component fixtures under
  `internal/wasm/runtime/plugin/testdata/components`

- [ ] Add failing tests for filename ordering, non-Wasm files, malformed
  components, missing metadata, duplicate identity, disabled plugins, unknown
  configuration, exact hash, additive structural compatibility, breaking
  imports, and generator format mismatch.
- [ ] Read bounded files and compile only after metadata and configured size
  checks. Component identity comes from validated exports, not filename.
- [ ] Compare the plugin's required structural manifest to the generated host
  manifest. Accept compatible additive host changes; reject changed or missing
  imported shapes with an exact path.
- [ ] Return diagnostics containing filename, plugin identity when known,
  contract hash, and incompatible operation without exposing another plugin's
  data.
- [ ] Run `go test ./internal/wasm/runtime/plugin -count=1`.
- [ ] Commit as `feat: discover compatible wasm plugins`.

---

### Task 3: Serialized executor and failure state

**Files:**

- Create: `internal/wasm/runtime/plugin/executor.go`
- Create: `internal/wasm/runtime/plugin/executor_test.go`
- Create: `internal/wasm/runtime/plugin/state.go`

- [ ] Add failing tests for concurrent call serialization, FIFO ordering,
  different-plugin concurrency, inline active-call re-entry, close while
  queued, and rejection after failure.
- [ ] Give each plugin one executor bound to its Wasmtime store. Top-level Gate
  callbacks queue; guest-to-host calls execute inline; same-plugin nested
  callbacks use the scoped active call and never enqueue behind themselves.
- [ ] Define states `loading`, `initializing`, `running`, `failed`, and
  `closed`, with legal atomic transitions and a single terminal cleanup path.
- [ ] Treat fuel, deadline, memory, invalid runtime state, and unrecoverable
  traps as plugin-fatal. Ordinary generated Gate API errors remain typed call
  results.
- [ ] Run executor tests with `-race` and a deadlock timeout.
- [ ] Commit as `feat: serialize wasm plugin execution`.

---

### Task 4: Initialization and Gate startup integration

**Files:**

- Create: `internal/wasm/runtime/plugin/manager.go`
- Create: `internal/wasm/runtime/plugin/manager_test.go`
- Modify: `pkg/edition/java/proxy/proxy.go`
- Modify: `pkg/edition/java/proxy/plugin.go`
- Modify: `pkg/gate/gate.go`
- Modify: `pkg/gate/gate_startup_test.go`

- [ ] Add a failing startup test proving component `init` receives the
  lifetime context and real `*proxy.Proxy` after `p.init()` but before the
  listener accepts connections.
- [ ] Construct the manager from validated Java configuration. Discover,
  compile, instantiate, and initialize plugins in deterministic filename
  order.
- [ ] Abort startup on read, compile, link, metadata, compatibility, duplicate,
  or initialization error. Close every previously initialized plugin in
  reverse order.
- [ ] Keep native `proxy.Plugins` behavior independent; it is not part of the
  generated component contract and existing native plugins are neither wrapped
  nor migrated.
- [ ] On proxy shutdown, cancel plugin lifetime contexts, allow generated
  shutdown callbacks to finish within Gate's shutdown wait, then close stores
  and resource tables exactly once.
- [ ] Add a listener probe proving no connection is accepted before every
  enabled component finishes `init`.
- [ ] Commit as `feat: initialize wasm plugins with Gate`.

---

### Task 5: Transactional typed events

**Files:**

- Create: `internal/wasm/runtime/plugin/events.go`
- Create: `internal/wasm/runtime/plugin/events_test.go`
- Generate event adapters through `internal/wasm/generate/callbacks.go`

- [ ] Add failing tests for registration priority, deterministic ordering,
  readable getters, mutable setters, borrowed-event expiry, nested event
  publication, successful patch commit, returned-error rollback, trap rollback,
  fuel/deadline rollback, and plugin-failure unsubscription.
- [ ] Generate one typed subscription operation and guest callback export for
  every public Gate event type. Event resources are borrowed for one dispatch.
- [ ] Stage mutations in a generated patch instead of mutating the live event.
  Apply the complete patch on successful callback return before the next
  subscriber; discard it on every error or trap.
- [ ] Track unsubscribe functions under plugin ownership. Failure and shutdown
  remove them even if guest cleanup traps.
- [ ] Exercise representative connection, login, mutable routing, command,
  plugin message, ready, pre-shutdown, and shutdown events.
- [ ] Commit as `feat: dispatch transactional events to wasm plugins`.

---

### Task 6: Commands, timers, and callback registrations

**Files:**

- Create: `internal/wasm/runtime/plugin/commands.go`
- Create: `internal/wasm/runtime/plugin/timers.go`
- Create: `internal/wasm/runtime/plugin/commands_test.go`
- Create: `internal/wasm/runtime/plugin/timers_test.go`

- [ ] Add failing tests for command registration and aliases, source
  resources, suggestions, callback errors, duplicate command cleanup, one-shot
  timers, recurring timers, timer limits, cancellation, serialization, and no
  guest execution while idle.
- [ ] Route generated function-value registrations to guest callback IDs.
  Retain each callback only as long as its owning registration.
- [ ] Implement host-owned one-shot and recurring timers. Timer firings enter
  the plugin executor; recurring timers never overlap for one plugin.
- [ ] Enforce 1,024 timers by default and per-plugin overrides. Close stops and
  drains timers before invalidating callback resources.
- [ ] Confirm the implementation exposes no WASI clock, thread, task, network,
  filesystem, process, environment, or random interface.
- [ ] Commit as `feat: add wasm commands and timers`.

---

### Task 7: Diagnostics, metrics, and runtime isolation

**Files:**

- Create: `internal/wasm/runtime/plugin/diagnostics.go`
- Create: `internal/wasm/runtime/plugin/metrics.go`
- Create: `internal/wasm/runtime/plugin/failure_test.go`

- [ ] Add failing tests that one plugin's trap removes only its registrations,
  timers, callbacks, and handles while another plugin and Gate continue.
- [ ] Normalize traps and backtraces with plugin and operation identity. Avoid
  component memory contents and other plugin data in logs.
- [ ] Add counters/histograms for load/init/call duration, traps, fuel,
  deadlines, memory, transfer, live handles, timers, queued callbacks, and
  failed plugins using Gate's existing telemetry conventions.
- [ ] Make fatal cleanup idempotent under concurrent callback, timer, and
  shutdown paths.
- [ ] Commit as `feat: isolate failed wasm plugins`.

---

### Task 8: Static release builds and containers

**Files:**

- Modify: `.goreleaser.yml`
- Modify: `.github/workflows/ci.yml`
- Modify: `Dockerfile`
- Modify: `Makefile`
- Create: release contract tests beside existing release workflow tests
- Modify: `AGENTS.md` only for durable release facts discovered during this task

- [ ] Add failing contract tests enumerating the release OS/architecture matrix
  and requiring each final Gate binary to contain the runtime version symbol
  and run without a sidecar/shared Wasmtime library.
- [ ] Explicitly reconcile Gate's current implicit GoReleaser architectures
  with Wasmtime-supported Rust targets. Do not silently publish a binary
  without plugin support under the same release; either build a tested native
  runtime for that target or explicitly remove the target with release-note
  coverage.
- [ ] Build native archives and final CGO binaries on matching platform
  toolchains. Use GNU Rust with MinGW Go on Windows. Publish from a separate
  least-privileged job using allowlisted matrix artifacts, preserving the
  repository's existing release-asset verification boundary.
- [ ] Update container builds to compile and link the Linux Rust archive for
  amd64 and arm64, and retain all existing managed-Bedrock smoke behavior.
- [ ] Add startup smoke components to Linux, macOS, Windows, and both container
  architectures. Inspect final dynamic dependencies and assert Wasmtime itself
  is not a shared runtime dependency.
- [ ] Update `build`/`releaser` job dependencies so release publication cannot
  bypass native Wasm tests.
- [ ] Commit as `build: ship the static wasm plugin runtime`.

---

### Task 9: End-to-end lifecycle verification

**Files:**

- Create: `pkg/gate/wasm_startup_test.go`
- Create fixtures under `internal/wasm/runtime/plugin/testdata/e2e`
- Modify: `.github/workflows/ci.yml`

- [ ] Boot Gate with two components and prove initialization, logging, proxy
  lookup, event mutation, command execution, timer callback, nested event
  re-entry, and clean shutdown.
- [ ] Add negative boots for incompatible contract, duplicate metadata,
  initialization error, denied WASI import, and memory limit.
- [ ] Add runtime failure tests proving Gate remains available after one
  plugin traps and that no failed-plugin registration survives.
- [ ] Run the complete suite with race detection where supported, 100 lifecycle
  repetitions, native platform jobs, container smoke jobs, and release contract
  tests.
- [ ] Commit as `test: verify wasm plugin lifecycle`.

---

## Completion checkpoint

Product runtime is complete only when startup ordering matches native `Init`,
all failure/cleanup behavior is deterministic, denied capabilities remain
unlinked, final release artifacts statically contain the runtime, and the
existing Gate startup/release tests remain green.
