# Wasm Built-In Plugin Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reorganize Gate's WebAssembly component support as an enabled-by-default built-in native Gate plugin under `internal/builtin/wasm` without changing its public WIT contract or exported configuration API.

**Architecture:** The standard CLI appends a Wasm `proxy.Plugin` after application-supplied native plugins. That plugin owns component discovery, initialization, shutdown, generated dispatch, and Wasmtime. The old exported component-manager API remains as a deprecated adapter into the same native plugin lifecycle so the refactor does not alter Gate's public Go or WIT contract.

**Tech Stack:** Go 1.26, `go/packages`, `go/types`, WIT Component Model, Rust 1.94, Wasmtime 47, CGO, GitHub Actions, Docker.

## Global Constraints

- The official Gate binary compiles in Wasm support and enables it by default.
- The default component directory remains `plugins/`.
- Missing and empty component directories are successful no-ops.
- Preserve `pkg/edition/java/config.Wasm`, its YAML/JSON keys, and generated public WIT content.
- Preserve `proxy.ComponentPluginManager` and `gate.WithComponentPlugins` as compatibility adapters; do not keep a second proxy lifecycle.
- Preserve component startup rollback, reverse-order shutdown, callback serialization, panic containment, and all resource limits.
- Do not leave compatibility aliases for old `internal/wasm` package paths.
- Component authors continue to use standard WIT tooling; this refactor does not add guest SDK generation.

---

### Task 1: Enable the Built-In by Default and Lock Down No-Op Discovery

**Files:**
- Modify: `pkg/edition/java/config/wasm_test.go`
- Modify: `pkg/edition/java/config/wasm.go`
- Modify: `internal/wasm/runtime/plugin/manager_test.go`

**Interfaces:**
- Consumes: `config.DefaultWasm`, `plugin.Manager.Start(context.Context, *proxy.Proxy) error`
- Produces: `config.DefaultWasm.Enabled == true`; missing and empty directories invoke no component loader

- [ ] **Step 1: Change the default test and add explicit no-op discovery coverage**

Replace the default assertion with:

```go
func TestDefaultWasmConfigIsSafeAndEnabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultWasm
	require.True(t, cfg.Enabled)
	require.Equal(t, "plugins", cfg.Directory)
	require.EqualValues(t, 128<<20, cfg.MemoryBytes)
	require.EqualValues(t, 16<<20, cfg.TransferBytes)
	require.EqualValues(t, 10_000_000, cfg.Fuel)
	require.EqualValues(t, 65_536, cfg.ResourceHandles)
	require.EqualValues(t, 1_024, cfg.TimerLimit)
	require.Equal(t, 5*time.Second, time.Duration(cfg.InitDeadline))
	require.Equal(t, 100*time.Millisecond, time.Duration(cfg.CallbackDeadline))
	require.Empty(t, cfg.Validate())
}
```

Add this manager test:

```go
func TestManagerTreatsMissingAndEmptyDirectoriesAsNoop(t *testing.T) {
	t.Parallel()

	for name, directory := range map[string]string{
		"missing": filepath.Join(t.TempDir(), "missing"),
		"empty":   t.TempDir(),
	} {
		t.Run(name, func(t *testing.T) {
			cfg := config.DefaultWasm
			cfg.Directory = directory
			loaded := false
			manager := newWithLoader(cfg, func(
				[]byte,
				*gatehost.Host,
				native.Limits,
			) (componentRuntime, error) {
				loaded = true
				return nil, errors.New("loader must not run")
			})

			require.NoError(t, manager.Start(context.Background(), &proxy.Proxy{}))
			require.False(t, loaded)
			require.NoError(t, manager.Close())
		})
	}
}
```

- [ ] **Step 2: Run the focused tests and confirm the new default fails**

Run:

```bash
go test ./pkg/edition/java/config ./internal/wasm/runtime/plugin -run 'TestDefaultWasm|TestManagerTreats' -count=1
```

Expected: `TestDefaultWasmConfigIsSafeAndEnabled` fails because `Enabled` is still false; the directory tests pass because `componentPaths` already handles `os.ErrNotExist` and empty results.

- [ ] **Step 3: Enable the default**

Change the first field of `DefaultWasm` to:

```go
var DefaultWasm = Wasm{
	Enabled:           true,
	Directory:         "plugins",
```

- [ ] **Step 4: Run configuration and manager tests**

Run:

```bash
go test ./pkg/edition/java/config ./internal/wasm/runtime/plugin -count=1
```

Expected: both packages pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/edition/java/config/wasm.go \
  pkg/edition/java/config/wasm_test.go \
  internal/wasm/runtime/plugin/manager_test.go
git commit -m "feat: enable built-in wasm plugins by default"
```

---

### Task 2: Route Wasm and Compatibility Managers Through Native Plugins

**Files:**
- Create: `internal/wasm/runtime/plugin/plugin.go`
- Create: `internal/wasm/runtime/plugin/plugin_test.go`
- Create: `cmd/gate/wasm_test.go`
- Modify: `cmd/gate/root.go`
- Modify: `pkg/edition/java/proxy/plugin.go`
- Modify: `pkg/edition/java/proxy/plugin_test.go`
- Modify: `pkg/edition/java/proxy/proxy.go`

**Interfaces:**
- Consumes: `Manager.Start`, `Manager.Close`, `proxy.Plugin`, `proxy.ShutdownEvent`
- Produces: `plugin.Plugin(config.Wasm) proxy.Plugin`; one native-plugin initialization loop; scoped standard-CLI registration after existing native plugins

- [ ] **Step 1: Write lifecycle tests for the Wasm native plugin**

Create `internal/wasm/runtime/plugin/plugin_test.go` with a fake lifecycle:

```go
package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

type fakeManagerLifecycle struct {
	ctx      context.Context
	proxy    *proxy.Proxy
	startErr error
	closed   int
	closeErr error
}

func (manager *fakeManagerLifecycle) Start(ctx context.Context, gateProxy *proxy.Proxy) error {
	manager.ctx = ctx
	manager.proxy = gateProxy
	return manager.startErr
}

func (manager *fakeManagerLifecycle) Close() error {
	manager.closed++
	return manager.closeErr
}

func TestPluginUsesNativeInitAndShutdownLifecycle(t *testing.T) {
	events := event.New()
	cfg := config.DefaultConfig
	gateProxy, err := proxy.New(proxy.Options{Config: &cfg, EventMgr: events})
	require.NoError(t, err)
	manager := &fakeManagerLifecycle{}
	builtin := plugin(manager)
	ctx := context.WithValue(context.Background(), struct{}{}, "same")

	require.Equal(t, "wasm", builtin.Name)
	require.NoError(t, builtin.Init(ctx, gateProxy))
	require.Same(t, ctx, manager.ctx)
	require.Same(t, gateProxy, manager.proxy)

	events.Fire(&proxy.ShutdownEvent{})
	events.Wait()
	require.Equal(t, 1, manager.closed)
}

func TestPluginReturnsManagerStartupFailure(t *testing.T) {
	expected := errors.New("start failed")
	manager := &fakeManagerLifecycle{startErr: expected}
	builtin := plugin(manager)

	require.ErrorIs(t, builtin.Init(context.Background(), &proxy.Proxy{}), expected)
	require.Zero(t, manager.closed)
}
```

- [ ] **Step 2: Replace the proxy test with one-loop compatibility coverage**

Replace `pkg/edition/java/proxy/plugin_test.go` with tests that construct
`Proxy{plugins: []Plugin{...}}`, verify native order, and verify the
`ComponentPluginManager` adapter:

```go
func TestInitPluginsRunsOneNativePluginSequence(t *testing.T) {
	var order []string
	gateProxy := &Proxy{plugins: []Plugin{
		{Name: "native", Init: func(context.Context, *Proxy) error {
			order = append(order, "native")
			return nil
		}},
		{Name: "wasm", Init: func(context.Context, *Proxy) error {
			order = append(order, "wasm")
			return nil
		}},
	}}

	require.NoError(t, gateProxy.initPlugins(context.Background()))
	require.Equal(t, []string{"native", "wasm"}, order)
}

func TestComponentManagerCompatibilityAdapterUsesNativeLifecycle(t *testing.T) {
	events := event.New()
	manager := &fixtureComponentPlugins{}
	gateProxy := &Proxy{
		event:   events,
		plugins: []Plugin{componentManagerPlugin(manager)},
	}
	ctx := context.Background()

	require.NoError(t, gateProxy.initPlugins(ctx))
	require.Same(t, ctx, manager.ctx)
	require.Same(t, gateProxy, manager.proxy)
	events.Fire(&ShutdownEvent{})
	events.Wait()
	require.Equal(t, 1, manager.closed)
}
```

Give `fixtureComponentPlugins` a `closed int` field incremented by `Close`.

- [ ] **Step 3: Add a standard-CLI registration test**

Create `cmd/gate/wasm_test.go`:

```go
package gate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
)

func TestWithBuiltinWasmAppendsAfterNativePluginsAndRestoresGlobal(t *testing.T) {
	previous := jproxy.Plugins
	jproxy.Plugins = []jproxy.Plugin{{Name: "application"}}
	t.Cleanup(func() { jproxy.Plugins = previous })
	expected := errors.New("stop")

	err := withBuiltinWasm(jconfig.DefaultWasm, func() error {
		require.Equal(t, []string{"application", "wasm"}, []string{
			jproxy.Plugins[0].Name,
			jproxy.Plugins[1].Name,
		})
		return expected
	})

	require.ErrorIs(t, err, expected)
	require.Equal(t, []jproxy.Plugin{{Name: "application"}}, jproxy.Plugins)
}
```

- [ ] **Step 4: Run the new tests and verify they fail to compile**

Run:

```bash
go test ./internal/wasm/runtime/plugin ./pkg/edition/java/proxy ./cmd/gate -run 'TestPlugin|TestInitPlugins|TestComponentManager|TestWithBuiltinWasm' -count=1
```

Expected: compilation fails because `plugin`, `componentManagerPlugin`,
`Proxy.plugins`, and `withBuiltinWasm` do not exist.

- [ ] **Step 5: Implement the native Wasm plugin**

Create `internal/wasm/runtime/plugin/plugin.go`:

```go
package plugin

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

type managerLifecycle interface {
	Start(context.Context, *proxy.Proxy) error
	Close() error
}

func Plugin(cfg config.Wasm) proxy.Plugin {
	return plugin(New(cfg))
}

func plugin(manager managerLifecycle) proxy.Plugin {
	return proxy.Plugin{
		Name: "wasm",
		Init: func(ctx context.Context, gateProxy *proxy.Proxy) error {
			if err := manager.Start(ctx, gateProxy); err != nil {
				return err
			}
			log := logr.FromContextOrDiscard(ctx).WithName("wasm")
			event.Subscribe(gateProxy.Event(), 0, func(*proxy.ShutdownEvent) {
				if err := manager.Close(); err != nil {
					log.Error(err, "error closing wasm plugins")
				}
			})
			return nil
		},
	}
}
```

- [ ] **Step 6: Collapse proxy initialization to one native sequence**

Add `plugins []Plugin` to `Proxy`. In `New`, clone the global slice and append
the compatibility adapter when configured:

```go
plugins := append([]Plugin(nil), Plugins...)
if options.ComponentPlugins != nil {
	plugins = append(plugins, componentManagerPlugin(options.ComponentPlugins))
}

p = &Proxy{
	// existing fields
	plugins: plugins,
}
```

Implement the adapter in `plugin.go` using `event.Subscribe` and
`logr.FromContextOrDiscard`, with name `"component"` and the same
`Start`/`Close` behavior as the Wasm plugin. Keep the exported interface and
option fields unchanged. Change `initPlugins` to iterate `p.plugins` and delete
its separate `p.componentPlugins.Start` branch. Remove the private
`componentPlugins` field and deferred close block.

Move the existing `defer p.Shutdown(...)` above `initPlugins` so a later native
plugin initialization failure still fires `ShutdownEvent` for plugins that
already initialized.

- [ ] **Step 7: Register the built-in only around standard CLI execution**

Add to `cmd/gate/root.go`:

```go
func withBuiltinWasm(cfg jconfig.Wasm, run func() error) error {
	previous := jproxy.Plugins
	jproxy.Plugins = append(append([]jproxy.Plugin(nil), previous...), wasmplugin.Plugin(cfg))
	defer func() {
		jproxy.Plugins = previous
	}()
	return run()
}
```

Import `jconfig`, `jproxy`, and the internal Wasm package. Replace
`gate.WithComponentPlugins(...)` in the standard action with:

```go
if err = withBuiltinWasm(cfg.Config.Wasm, func() error {
	return gate.Start(c.Context, startOpts...)
}); err != nil {
	return cli.Exit(fmt.Errorf("error running Gate: %w", err), 1)
}
```

Keep `gate.WithComponentPlugins` and the exported option fields so existing Go
callers and generated WIT remain compatible; their manager is converted to a
native plugin by `proxy.New`.

- [ ] **Step 8: Run lifecycle tests**

Run:

```bash
go test ./internal/wasm/runtime/plugin ./pkg/edition/java/proxy ./pkg/gate ./cmd/gate -count=1
```

Expected: all packages pass.

- [ ] **Step 9: Commit**

```bash
git add cmd/gate \
  internal/wasm/runtime/plugin \
  pkg/edition/java/proxy \
  pkg/gate
git commit -m "refactor: run wasm as a built-in native plugin"
```

---

### Task 3: Move the Go Subsystem into the Built-In Vertical Slice

**Files:**
- Move: `internal/wasm/analyze` → `internal/builtin/wasm/codegen/analyze`
- Move: `internal/wasm/model` → `internal/builtin/wasm/codegen/model`
- Move: `internal/wasm/generate` → `internal/builtin/wasm/codegen/generate`
- Move: `internal/wasm/cmd/gate-wasm-gen` → `internal/builtin/wasm/codegen/cmd/gate-wasm-gen`
- Move: `internal/wasm/api` → `internal/builtin/wasm/generated`
- Move: `internal/wasm/runtime/gatehost` → `internal/builtin/wasm/host`
- Move: `internal/wasm/runtime/abi` → `internal/builtin/wasm/runtime/abi`
- Move: `internal/wasm/runtime/dispatch` → `internal/builtin/wasm/runtime/dispatch`
- Move: `internal/wasm/runtime/resources` → `internal/builtin/wasm/runtime/resources`
- Move: `internal/wasm/runtime/wire` → `internal/builtin/wasm/runtime/wire`
- Move: `internal/wasm/runtime/plugin/*.go` → `internal/builtin/wasm/*.go`

**Interfaces:**
- Consumes: the behavior established in Tasks 1–2
- Produces: `go.minekube.com/gate/internal/builtin/wasm` and its internal codegen, generated, host, and runtime packages; no live Go import under `internal/wasm`

- [ ] **Step 1: Record public contract hashes before moving**

Run:

```bash
sha256sum wasm/wit/gate.wit wasm/wit/contract.json > /tmp/gate-wasm-contract-before.sha256
```

Expected: two hashes are recorded for post-move comparison.

- [ ] **Step 2: Move the Go directories and files**

Run:

```bash
mkdir -p internal/builtin/wasm/codegen/cmd internal/builtin/wasm/runtime
git mv internal/wasm/analyze internal/builtin/wasm/codegen/analyze
git mv internal/wasm/model internal/builtin/wasm/codegen/model
git mv internal/wasm/generate internal/builtin/wasm/codegen/generate
git mv internal/wasm/cmd/gate-wasm-gen internal/builtin/wasm/codegen/cmd/gate-wasm-gen
git mv internal/wasm/api internal/builtin/wasm/generated
git mv internal/wasm/runtime/gatehost internal/builtin/wasm/host
git mv internal/wasm/runtime/abi internal/builtin/wasm/runtime/abi
git mv internal/wasm/runtime/dispatch internal/builtin/wasm/runtime/dispatch
git mv internal/wasm/runtime/resources internal/builtin/wasm/runtime/resources
git mv internal/wasm/runtime/wire internal/builtin/wasm/runtime/wire
git mv internal/wasm/runtime/plugin/*.go internal/builtin/wasm/
rmdir internal/wasm/runtime/plugin internal/wasm/cmd
```

- [ ] **Step 3: Update package and import paths**

Change package declarations in the moved manager, executor, plugin, and their
tests from `package plugin` to `package wasm`.

Apply these exact import-path mappings throughout non-historical source,
generated Go emitters, tests, `cmd/gate`, and maintainer docs:

```text
internal/wasm/analyze          -> internal/builtin/wasm/codegen/analyze
internal/wasm/model            -> internal/builtin/wasm/codegen/model
internal/wasm/generate         -> internal/builtin/wasm/codegen/generate
internal/wasm/cmd/gate-wasm-gen -> internal/builtin/wasm/codegen/cmd/gate-wasm-gen
internal/wasm/api              -> internal/builtin/wasm/generated
internal/wasm/runtime/gatehost -> internal/builtin/wasm/host
internal/wasm/runtime/abi      -> internal/builtin/wasm/runtime/abi
internal/wasm/runtime/dispatch -> internal/builtin/wasm/runtime/dispatch
internal/wasm/runtime/resources -> internal/builtin/wasm/runtime/resources
internal/wasm/runtime/wire     -> internal/builtin/wasm/runtime/wire
internal/wasm/runtime/plugin   -> internal/builtin/wasm
```

Package names inside `generated`, `host`, and the later `wasmtime` directory
remain `api`, `gatehost`, and `native` respectively; callers should use explicit
aliases where the directory basename differs.

- [ ] **Step 4: Update generator-owned paths**

Change the generated Go imports in `callbacks.go` and `go_dispatch.go` to:

```go
"go.minekube.com/gate/internal/builtin/wasm/runtime/dispatch"
```

Change `internal/builtin/wasm/generated/generate.go` to:

```go
// Package api contains Gate's generated language-neutral WebAssembly contract.
package api

//go:generate go run ../codegen/cmd/gate-wasm-gen generate -repo ../../../.. -out . -native-out ../wasmtime/host/src/generated -public-out ../../../../wasm/wit
```

Update generator command defaults and tests to use the new `generated` and
`wasmtime/host/src/generated` locations.

- [ ] **Step 5: Format and test all moved Go packages without native Rust**

Run:

```bash
gofmt -w internal/builtin/wasm cmd/gate
go test ./internal/builtin/wasm/... ./cmd/gate ./pkg/edition/java/proxy ./pkg/gate -count=1
```

Expected: every moved Go package and lifecycle integration passes under the
default non-`wasm_native` build.

- [ ] **Step 6: Confirm only the not-yet-moved native package remains under the old tree**

Run:

```bash
rg -n 'go\\.minekube\\.com/gate/internal/wasm|internal/wasm' \
  --glob '*.go' --glob '!docs/superpowers/**' .
```

Expected: matches are limited to imports of
`go.minekube.com/gate/internal/wasm/runtime/native` from the moved manager and
host, plus Go files physically inside that native workspace. Task 4 removes
those final references.

- [ ] **Step 7: Commit**

```bash
git add internal/builtin internal/wasm cmd/gate .web/docs/developers/wasm-plugins.md
git commit -m "refactor: colocate wasm under built-in plugins"
```

---

### Task 4: Move Wasmtime and Update Build, Rust, Release, and Documentation Paths

**Files:**
- Move: `internal/wasm/runtime/native` → `internal/builtin/wasm/wasmtime`
- Modify: `internal/builtin/wasm/wasmtime/guest-fixture/build.rs`
- Modify: `internal/builtin/wasm/wasmtime/guest-fixture/src/lib.rs`
- Modify: `internal/builtin/wasm/wasmtime/componentize/src/lib.rs`
- Modify: `internal/builtin/wasm/wasmtime/host/src/generated/bindings.rs` through regeneration
- Modify: `internal/builtin/wasm/wasmtime/host/tests/generated_values.rs`
- Modify: `internal/builtin/wasm/wasmtime/example_integration_test.go`
- Modify: `internal/builtin/wasm/wasmtime/release_contract_test.go`
- Modify: `Makefile`
- Modify: `Dockerfile`
- Modify: `.gitignore`
- Modify: `.github/workflows/ci.yml`
- Modify: `.web/docs/developers/wasm-plugins.md`
- Modify: `internal/builtin/wasm/wasmtime/README.md`

**Interfaces:**
- Consumes: `internal/builtin/wasm/generated`, public `wasm/wit`
- Produces: Rust workspace and CGO archive rooted at `internal/builtin/wasm/wasmtime`; release artifacts use the moved manifest

- [ ] **Step 1: Move the native workspace**

Run:

```bash
git mv internal/wasm/runtime/native internal/builtin/wasm/wasmtime
rmdir internal/wasm/runtime internal/wasm
```

- [ ] **Step 2: Update Rust-relative generated contract paths**

Use these exact replacements:

```text
wasmtime/guest-fixture: ../../../api -> ../../generated
wasmtime/componentize tests: ../../../generate -> ../../codegen/generate
wasmtime/componentize tests: ../../../api -> ../../generated
wasmtime/host generated bindgen path: ../../../api -> ../../generated
wasmtime/host/tests/generated_values.rs: ../../../../api -> ../../../generated
```

Also change the `wit_bindgen::generate!` path emitted by
`codegen/generate/rust_dispatch.go` from `"../../../api"` to
`"../../generated"` before regeneration.

Change the public Rust example component path in
`example_integration_test.go` from four parent traversals to three:

```go
filepath.Join("..", "..", "..", ".examples", "wasm", "rust", "gate-rust-example.component.wasm")
```

Change the repository root in `release_contract_test.go` to:

```go
root := filepath.Join("..", "..", "..")
```

- [ ] **Step 3: Update build and release paths**

Set:

```make
WASM_NATIVE_DIR := internal/builtin/wasm/wasmtime
```

Update Make targets to test `./internal/builtin/wasm/wasmtime`, run
`./internal/builtin/wasm/codegen/cmd/gate-wasm-gen`, write Go artifacts to
`internal/builtin/wasm/generated`, and write Rust artifacts to
`internal/builtin/wasm/wasmtime/host/src/generated`. Reduce the Rust example
relative traversal from `../../../../` to `../../../`.

Update:

```text
Dockerfile cargo directory -> internal/builtin/wasm/wasmtime
.gitignore target path -> internal/builtin/wasm/wasmtime/target/
.gitignore artifact path -> internal/builtin/wasm/wasmtime/artifacts/*.wasm
CI Rust cache workspace -> internal/builtin/wasm/wasmtime -> target
CI uploaded/released manifest -> internal/builtin/wasm/generated/manifest.json
```

- [ ] **Step 4: Update current documentation links**

Point `.web/docs/developers/wasm-plugins.md` at
`internal/builtin/wasm/generated`. Update the Wasmtime README's relative link
to the generated WIT. Leave dated historical plans unchanged.

- [ ] **Step 5: Regenerate synchronized artifacts at their new paths**

Run:

```bash
make wasm-api-generate
make wasm-api-check
```

Expected: generation and drift checking succeed using only the new tree.

- [ ] **Step 6: Verify the public WIT contract is byte-identical**

Run:

```bash
sha256sum -c /tmp/gate-wasm-contract-before.sha256
```

Expected: both `wasm/wit/gate.wit` and `wasm/wit/contract.json` report `OK`.

- [ ] **Step 7: Run moved Go and Rust checks**

Run:

```bash
go test ./internal/builtin/wasm/... -count=1
cargo test --manifest-path internal/builtin/wasm/wasmtime/Cargo.toml --workspace
cargo fmt --manifest-path internal/builtin/wasm/wasmtime/Cargo.toml --all -- --check
cargo clippy --manifest-path internal/builtin/wasm/wasmtime/Cargo.toml --workspace --all-targets -- -D warnings
```

Expected: all commands pass.

- [ ] **Step 8: Commit**

```bash
git add .gitignore .github/workflows/ci.yml Dockerfile Makefile \
  .web/docs/developers/wasm-plugins.md \
  internal/builtin/wasm/wasmtime \
  internal/builtin/wasm/generated \
  wasm/wit
git commit -m "build: relocate embedded wasmtime workspace"
```

---

### Task 5: Full Verification and Contract Guard

**Files:**
- Modify if required by failures: files already touched in Tasks 1–4

**Interfaces:**
- Consumes: complete built-in Wasm vertical slice
- Produces: generation-clean, Go-tested, race-tested, Rust-tested, lint-clean, release-build-ready branch

- [ ] **Step 1: Check final layout and stale paths**

Run:

```bash
test ! -d internal/wasm
rg -uu -n 'internal/wasm' \
  --glob '!.git/**' \
  --glob '!docs/superpowers/**'
```

Expected: `internal/wasm` does not exist; remaining matches are absent or are
deliberate dated design-history references.

- [ ] **Step 2: Verify generation and focused race-sensitive packages**

Run:

```bash
make wasm-api-check
go test -race ./internal/builtin/wasm ./internal/builtin/wasm/runtime/... ./internal/builtin/wasm/host -count=1
```

Expected: generated artifacts are current and race-sensitive packages pass.

- [ ] **Step 3: Run the complete Go suite**

Run:

```bash
make test
```

Expected: formatting, vetting, docs shell tests, installer tests, and `go test ./...` pass.

- [ ] **Step 4: Run the embedded native integration suite**

Run:

```bash
make wasm-native-test
make wasm-rust-example-test
```

Expected: the Rust fixture component and public Rust example build,
componentize, load, receive real context/proxy resources, and initialize.

- [ ] **Step 5: Run repository lint and diff checks**

Run:

```bash
make lint
git diff --check
git status --short
```

Expected: lint and diff checks pass; status contains only intentional refactor
changes not yet committed.

- [ ] **Step 6: Commit any verification-only corrections**

If verification required corrections, commit only those corrections:

```bash
git add -A
git commit -m "fix: complete wasm built-in plugin verification"
```

If no corrections were required, do not create an empty commit.

- [ ] **Step 7: Record final evidence**

Run:

```bash
git log -6 --oneline
git status --short
```

Expected: the design, behavior, lifecycle, relocation, and build commits are
visible and the worktree is clean.
