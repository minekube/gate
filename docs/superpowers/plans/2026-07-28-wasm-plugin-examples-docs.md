# Gate WebAssembly Language-Neutral Examples and Documentation Plan

> **Execution requirement:** Begin once the product runtime contract is stable.
> Use executing-plans and test-driven-development. Examples must consume
> generated WIT through upstream language tooling; do not create Gate-owned
> language SDKs.

**Goal:** Publish the generated WIT contract and provide minimal Go and Rust
components demonstrating the same initialization experience as native plugins:
receive context and proxy, call Gate APIs, mutate an event, register a command,
and schedule a timer.

---

### Task 1: Publishable contract bundle

**Files:**

- Create: `wasm/wit/gate.wit`
- Create: `wasm/wit/contract.json`
- Create: `wasm/wit/manifest.json`
- Create: `wasm/README.md`
- Modify: `internal/wasm/cmd/gate-wasm-gen/main.go`
- Modify: `Makefile`

- [ ] Add a failing drift test requiring the public `wasm/wit` bundle to be
  byte-identical to the canonical generated artifacts.
- [ ] Extend generation to update internal and public contract bundles
  atomically from the same canonical model.
- [ ] Document contract hash, generator format, structural compatibility, and
  the rule that breaking Gate changes require plugin authors to regenerate
  bindings and rebuild.
- [ ] State explicitly that WIT is language-neutral and Gate does not maintain
  Go, Rust, TypeScript, or other SDK wrappers.
- [ ] Commit as `docs: publish the Gate wasm contract`.

---

### Task 2: Rust example component using upstream WIT bindgen

**Files:**

- Create: `.examples/wasm/rust/Cargo.toml`
- Create: `.examples/wasm/rust/src/lib.rs`
- Create: `.examples/wasm/rust/README.md`
- Create: `.examples/wasm/rust/tests/component.rs`
- Modify: `internal/wasm/runtime/native/Cargo.toml`

- [ ] Add a failing build/run test that consumes `wasm/wit/gate.wit` with the
  pinned upstream `wit-bindgen` crate.
- [ ] Implement metadata and `init(context, proxy)`, log through Gate, inspect
  context cancellation, look up a configured server, mutate a routing event,
  register a command, and schedule a recurring timer.
- [ ] Keep example helpers local and minimal; do not wrap the generated API
  into a Gate-specific SDK.
- [ ] Componentize with the repository's pinned tool and run it through the
  production plugin manager.
- [ ] Commit as `example: add a Rust wasm plugin`.

---

### Task 3: Go example component using upstream WIT bindgen

**Files:**

- Create: `.examples/wasm/go/go.mod`
- Create: `.examples/wasm/go/plugin.go`
- Create: `.examples/wasm/go/README.md`
- Create: `.examples/wasm/go/component_test.go`
- Add a reproducible Go component build helper under
  `internal/wasm/cmd/` only if upstream tooling requires orchestration

- [ ] Add a failing build/run test using the upstream Go WIT binding workflow,
  with no copied Gate-generated Go SDK.
- [ ] Implement the same observable initialization, logging, lookup, event,
  command, and timer behavior as the Rust example.
- [ ] Document the event-driven reactor constraint: guest work runs only while
  Gate invokes an export; recurring work uses Gate timer registrations rather
  than free-running guest goroutines.
- [ ] Pin every build tool and make a clean checkout build reproducible without
  globally installed `cargo-component`, `wasm-tools`, or `wit-bindgen`.
- [ ] Commit as `example: add a Go wasm plugin`.

---

### Task 4: User documentation

**Files:**

- Create: `.web/docs/developers/wasm-plugins.md`
- Modify the relevant `.web/docs` navigation file
- Modify: `README.md` if its extension section links developer guides
- Modify: `config.yml`

- [ ] Add documentation tests for internal links, code paths, configuration
  keys, and commands.
- [ ] Explain the experience first: place a component in `plugins/`, enable
  Wasm, Gate calls `init(context, proxy)` before accepting connections, and
  the component calls the same generated Gate contract from any WIT-supported
  language.
- [ ] Cover metadata, deterministic load order, startup failure, structural
  compatibility, rebuilds after breaking changes, resources and borrowed
  lifetimes, event transactions, commands, timers, limits, logging, denied
  capabilities, and failure isolation.
- [ ] Include concise Rust and Go build/run walkthroughs that link to the
  examples instead of duplicating generated bindings.
- [ ] Avoid the phrase “generate SDKs.” Say “generate standard language
  bindings from Gate's WIT contract” to preserve the language-neutral model the
  user approved.
- [ ] Commit as `docs: explain language-neutral wasm plugins`.

---

### Task 5: CI examples and release bundle

**Files:**

- Modify: `.github/workflows/ci.yml`
- Modify: `.goreleaser.yml`
- Modify: `Makefile`

- [ ] Add a CI job that generates/checks the WIT bundle, builds both examples
  from a clean tool cache, loads both into Gate, and asserts their observable
  behaviors.
- [ ] Package `wasm/wit` and example source as a versioned release artifact
  alongside Gate binaries. Include its checksum in the existing release
  verification.
- [ ] Verify no example imports an internal Gate host package or a
  Gate-maintained language wrapper.
- [ ] Run `make test`, native runtime tests, docs checks, example end-to-end
  tests, `git diff --check`, and the release contract tests.
- [ ] Commit as `ci: verify wasm plugin examples`.

---

## Completion checkpoint

Documentation is complete when a plugin author can start from only the
published WIT bundle, standard upstream tooling, and one of the examples;
build a component; place it in Gate's plugin directory; and observe
initialization plus Gate API calls without installing or learning a
Gate-specific language SDK.
