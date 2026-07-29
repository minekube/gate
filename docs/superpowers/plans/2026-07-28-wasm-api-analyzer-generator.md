# Gate WebAssembly API Analyzer and WIT Generator Plan

> **Execution requirement:** Use the executing-plans and test-driven-development
> skills task by task. Every task starts with a failing test and ends in a
> focused commit.

**Goal:** Generate a deterministic, language-neutral WIT contract and complete
coverage manifest from Gate's externally importable Go API, with no
hand-maintained wrapper list and no silently omitted declaration.

**Architecture:** `go/packages` loads Gate with syntax, types, type information,
and dependencies. An analyzer converts resolved declarations into a sorted
canonical model. A separate generator consumes only that model to emit WIT,
contract metadata, and the coverage manifest. The analyzer owns Go semantics;
renderers never inspect source directly.

**Scope:** All externally importable packages under
`go.minekube.com/gate/api/...` and `go.minekube.com/gate/pkg/...`. Today
`api/...` contains protobuf sources but no Go packages; the empty root remains
part of the discovery rule. Any path containing `/internal/` is excluded by
Go's visibility rule. `proxy.Plugin` and `proxy.Plugins` are the only initial
explicit public exclusions.

**Generated artifacts:**

```text
internal/wasm/api/gate.wit
internal/wasm/api/manifest.json
internal/wasm/api/contract.json
```

**Source layout:**

```text
internal/wasm/
  analyze/
    load.go
    declarations.go
    lower.go
    names.go
    policy.go
    testdata/module/...
  model/
    model.go
    kinds.go
    normalize.go
  generate/
    wit.go
    manifest.go
    contract.go
    compatibility.go
  cmd/gate-wasm-gen/main.go
  api/generate.go
```

---

### Task 1: Package discovery and fail-closed declaration coverage

**Files:**

- Create: `internal/wasm/analyze/load.go`
- Create: `internal/wasm/analyze/declarations.go`
- Create: `internal/wasm/analyze/policy.go`
- Create: `internal/wasm/analyze/load_test.go`
- Create: `internal/wasm/analyze/testdata/module/go.mod`
- Create fixture packages under
  `internal/wasm/analyze/testdata/module/api`, `pkg/public`, and
  `pkg/internal/hidden`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] Add a failing fixture test that requires exported types, aliases,
  constants, variables, functions, and methods to be discovered, while
  unexported declarations, tests, `cmd`, and any `/internal/` package are
  absent.
- [ ] Load with `packages.NeedName`, `NeedFiles`, `NeedCompiledGoFiles`,
  `NeedSyntax`, `NeedTypes`, `NeedTypesInfo`, `NeedImports`, and `NeedDeps`.
  Treat every package or type-check diagnostic as a generation failure.
- [ ] Match scope by canonical import path rather than filesystem location.
  Sort packages and declarations by stable package-qualified identity.
- [ ] Encode the two native bootstrap exclusions in `policy.go` with exact
  identities and human-readable reasons. A policy entry that no longer matches
  a declaration is an error, preventing stale exclusions.
- [ ] Add a repository test proving the production load finds no `cmd` or
  internal packages and finds both `proxy.Proxy` and its exported method set.
- [ ] Run `go test ./internal/wasm/analyze -run TestLoad -count=1`.
- [ ] Commit as `feat: discover the public Gate API for wasm`.

---

### Task 2: Canonical model and collision-proof names

**Files:**

- Create: `internal/wasm/model/model.go`
- Create: `internal/wasm/model/kinds.go`
- Create: `internal/wasm/model/normalize.go`
- Create: `internal/wasm/model/model_test.go`
- Create: `internal/wasm/analyze/names.go`
- Create: `internal/wasm/analyze/names_test.go`

- [ ] Add failing tests for deterministic ordering, canonical identity,
  package-name collisions, initialisms, punctuation, keywords, and two Go
  declarations that normalize to the same WIT name.
- [ ] Model packages, declarations, callables, parameters, types, fields,
  constants, ownership, lifetime, nilability, errors, callback direction,
  docs, source identity, and dependency edges without retaining `go/types`
  objects.
- [ ] Derive WIT kebab-case names from full import paths. Resolve ordinary
  cross-package collisions with a stable package-path suffix and fail if two
  declarations still map to the same canonical WIT identity.
- [ ] Normalize and sort every slice before hashing or rendering. Reject a
  duplicate canonical identity instead of taking the first filesystem result.
- [ ] Round-trip the canonical model through JSON in tests to prove generators
  do not depend on compiler-only state.
- [ ] Run `go test ./internal/wasm/model ./internal/wasm/analyze -count=1`.
- [ ] Commit as `feat: model the canonical wasm API contract`.

---

### Task 3: Complete Go type lowering

**Files:**

- Create: `internal/wasm/analyze/lower.go`
- Create: `internal/wasm/analyze/lower_test.go`
- Add fixture packages under
  `internal/wasm/analyze/testdata/module/pkg/shapes`

- [ ] Add table-driven failing tests for booleans; signed and unsigned
  fixed-width numbers; architecture-sized integers; strings; bytes; named
  types; aliases; constants; arrays; slices; maps; structs; pointers;
  interfaces; errors; functions; channels; contexts; timestamps; durations;
  `any`; variadics; embedded fields; and recursive types.
- [ ] Lower immutable, fully public value graphs to WIT scalars, records,
  variants, options, lists, tuples, enums, and flags. Treat slices, maps, and
  arrays as copied snapshots.
- [ ] Lower identity-bearing, mutable, recursive, unsafe, externally opaque, or
  non-inspectable types to generation-checked typed resources. Never expose a
  pointer value or memory address.
- [ ] Lower `error` as a typed `result` error when the concrete error family is
  known, with a common `gate-error` fallback carrying stable kind, message, and
  operation identity.
- [ ] Model function values as guest callback resources, channels as typed
  send/receive/close resources, `context.Context` as a borrowed cancellable
  resource, time values as canonical records, and `any` as a tagged dynamic
  value plus opaque-resource escape hatch.
- [ ] Discover instantiated generic uses from resolved signatures and syntax,
  then emit stable non-generic monomorphizations. Record an exported generic
  declaration as covered only when every reachable repository instantiation is
  represented; otherwise fail with the type-argument path.
- [ ] Break recursive value expansion at the first identity-bearing edge and
  report a precise declaration/type path for a shape that cannot be represented
  without semantic loss.
- [ ] Run
  `go test ./internal/wasm/analyze -run 'TestLower|TestGeneric' -count=1`.
- [ ] Commit as `feat: lower Gate Go types into component types`.

---

### Task 4: Callable, variable, constant, and callback modeling

**Files:**

- Modify: `internal/wasm/analyze/declarations.go`
- Modify: `internal/wasm/analyze/lower.go`
- Create: `internal/wasm/analyze/callables_test.go`

- [ ] Add failing tests for package functions, value and pointer receiver
  methods, promoted methods, constructors, variadics, multiple results,
  `(..., error)` results, getters/setters for variables, constants, and
  function-valued parameters/results.
- [ ] Represent pointer/interface receiver methods as resource methods. Lower
  pure value receiver methods to package operations with an explicit copied
  receiver when WIT cannot attach behavior to a record.
- [ ] Generate read access for every exported variable and write access only
  where assignment is legal and representable. Preserve named constant type
  and exact value; use enum/flags only when lossless.
- [ ] Give every guest callback signature a canonical callback identity and
  record whether it is invoked synchronously, may re-enter, returns a value, or
  is registration-owned.
- [ ] Detect exported event types and annotate them for later generated typed
  subscription entry points without coupling the canonical model to the event
  manager implementation.
- [ ] Assert that every discovered declaration ends in exactly one coverage
  state: represented or explicitly excluded. Missing, duplicate, or
  partially-lowered declarations fail analysis.
- [ ] Run
  `go test ./internal/wasm/analyze -run 'TestCallable|TestCoverage' -count=1`.
- [ ] Commit as `feat: model callable Gate wasm operations`.

---

### Task 5: Deterministic WIT, metadata, and compatibility generation

**Files:**

- Create: `internal/wasm/generate/wit.go`
- Create: `internal/wasm/generate/manifest.go`
- Create: `internal/wasm/generate/contract.go`
- Create: `internal/wasm/generate/compatibility.go`
- Create: `internal/wasm/generate/generate_test.go`
- Modify: `internal/wasm/runtime/native/componentize/Cargo.toml`
- Modify: `internal/wasm/runtime/native/componentize/src/lib.rs`

- [ ] Add failing goldens for package-grouped WIT interfaces, plugin metadata,
  `init(context, proxy)`, callbacks, resources, ownership, and errors.
- [ ] Render only from the canonical model. Include the standard generated-file
  warning and generator format version in every artifact.
- [ ] Add a pinned `wit-parser` validation function to the existing Rust
  componentize crate and exercise every generated WIT golden through it.
- [ ] Generate `manifest.json` with one entry per discovered declaration,
  lowering details, dependencies, documentation, and explicit exclusion
  reasons.
- [ ] Generate `contract.json` with generator format, canonical model hash,
  WIT hash, and the structural import manifest used at component load time.
- [ ] Implement structural compatibility tests: additive declarations pass;
  removal or changes to functions, methods, fields, ownership, callbacks,
  errors, and record shapes fail with the first incompatible path.
- [ ] Generate twice from shuffled package and declaration inputs and require
  byte-identical WIT, JSON, and hashes.
- [ ] Run
  `go test ./internal/wasm/generate ./internal/wasm/analyze ./internal/wasm/model -count=1`
  and the focused Rust WIT parser test.
- [ ] Commit as `feat: generate the language-neutral Gate WIT contract`.

---

### Task 6: Repository generator and committed full-API contract

**Files:**

- Create: `internal/wasm/cmd/gate-wasm-gen/main.go`
- Create: `internal/wasm/cmd/gate-wasm-gen/main_test.go`
- Create: `internal/wasm/api/generate.go`
- Generate: `internal/wasm/api/gate.wit`
- Generate: `internal/wasm/api/manifest.json`
- Generate: `internal/wasm/api/contract.json`
- Modify: `Makefile`

- [ ] Add a failing CLI test for `generate`, `check`, `-repo`, and `-out`
  behavior, including useful diagnostics for an unsupported type path.
- [ ] Implement an atomic generator: build all outputs in memory, validate
  them, then replace files. A failed generation leaves committed artifacts
  unchanged.
- [ ] Add `//go:generate go run ../cmd/gate-wasm-gen generate` and Make targets
  `wasm-api-generate` and `wasm-api-check`. `check` generates into a temporary
  directory and reports a per-file diff without modifying the worktree.
- [ ] Run the generator against the real repository. Resolve every diagnostic
  through general lowering rules; do not add declaration-specific wrappers.
  Only the approved native bootstrap exclusions may remain.
- [ ] Assert the coverage totals equal discovered represented declarations plus
  the two exclusions, and assert zero unrepresented declarations.
- [ ] Run `make wasm-api-generate`, `make wasm-api-check`, and
  `go test ./internal/wasm/...`.
- [ ] Commit generated artifacts and source as
  `feat: generate Gate's complete wasm contract`.

---

### Task 7: CI drift and breaking-change guard

**Files:**

- Modify: `.github/workflows/ci.yml`
- Create: `internal/wasm/api/contract_guard_test.go`
- Modify: `internal/wasm/runtime/native/README.md`

- [ ] Add a failing repository test that mutates a fixture signature and proves
  the contract hash changes and structural compatibility reports the exact
  breaking path.
- [ ] Add `make wasm-api-check` to the lint path so a public API change without
  regenerated artifacts fails CI.
- [ ] Upload the generated contract and manifest as CI artifacts for review.
- [ ] Document that WIT is the language-neutral interface, not a
  Gate-maintained SDK, and that a breaking Gate contract requires plugin
  regeneration and rebuild.
- [ ] Run `make wasm-api-check`, `make test`, `cargo test --manifest-path
  internal/wasm/runtime/native/Cargo.toml --workspace`, and `git diff --check`.
- [ ] Commit as `ci: enforce generated wasm API coverage`.

---

## Completion checkpoint

Do not start dispatcher generation until:

- the real Gate repository generates with zero unrepresented declarations;
- only `proxy.Plugin` and `proxy.Plugins` are explicitly excluded;
- WIT parses successfully;
- regeneration is byte deterministic;
- the committed-artifact drift check is green.
