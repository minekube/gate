package wasm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/builtin/wasm/host"
	"go.minekube.com/gate/internal/wasm/runtime/native"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

type fakeRuntime struct {
	metadata native.Metadata
	init     func() error
	close    func() error
}

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

func (runtime *fakeRuntime) Metadata() (native.Metadata, error) {
	return runtime.metadata, nil
}

func (runtime *fakeRuntime) Init(uint64, uint64) error {
	if runtime.init != nil {
		return runtime.init()
	}
	return nil
}

func (runtime *fakeRuntime) SetDeadline(time.Duration) error {
	return nil
}

func (runtime *fakeRuntime) InvokeCallback(uint32, uint64, []byte) ([]byte, error) {
	return nil, nil
}

func (runtime *fakeRuntime) Close() error {
	if runtime.close != nil {
		return runtime.close()
	}
	return nil
}

func TestManagerLoadsComponentsInFilenameOrderAndSkipsDisabledIdentity(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "b.wasm"), []byte("bravo"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "a.wasm"), []byte("alpha"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "ignored.txt"), []byte("ignore"), 0o600))
	cfg := config.DefaultWasm
	cfg.Enabled = true
	cfg.Directory = directory
	cfg.Plugins = map[string]config.WasmPlugin{
		"bravo": {Disabled: true},
	}
	var loaded []string
	loader := func(
		component []byte,
		_ *gatehost.Host,
		_ native.Limits,
	) (componentRuntime, error) {
		name := string(component)
		loaded = append(loaded, name)
		return &fakeRuntime{metadata: native.Metadata{
			Name: name, Version: "1", ContractHash: "hash", GeneratorFormat: 1,
		}}, nil
	}
	manager := newWithLoader(cfg, loader)

	err := manager.Start(context.Background(), &proxy.Proxy{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	require.Equal(t, []string{"alpha", "bravo"}, loaded)
	require.Equal(t, []string{"alpha"}, manager.Names())
}

func TestManagerRejectsDuplicateIdentityAndClosesInitializedPluginsInReverse(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	for _, name := range []string{"a.wasm", "b.wasm", "c.wasm"} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(name), 0o600))
	}
	cfg := config.DefaultWasm
	cfg.Enabled = true
	cfg.Directory = directory
	var closed []string
	loader := func(
		component []byte,
		_ *gatehost.Host,
		_ native.Limits,
	) (componentRuntime, error) {
		file := string(component)
		identity := file
		if file == "c.wasm" {
			identity = "a.wasm"
		}
		return &fakeRuntime{
			metadata: native.Metadata{Name: identity},
			close: func() error {
				closed = append(closed, file)
				return nil
			},
		}, nil
	}
	manager := newWithLoader(cfg, loader)

	err := manager.Start(context.Background(), &proxy.Proxy{})
	require.ErrorContains(t, err, `duplicate wasm plugin identity "a.wasm"`)
	require.Equal(t, []string{"c.wasm", "b.wasm", "a.wasm"}, closed)
}

func TestManagerClosesPriorPluginsWhenInitializationFails(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	for _, name := range []string{"a.wasm", "b.wasm"} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(name), 0o600))
	}
	cfg := config.DefaultWasm
	cfg.Enabled = true
	cfg.Directory = directory
	var closed []string
	loader := func(
		component []byte,
		_ *gatehost.Host,
		_ native.Limits,
	) (componentRuntime, error) {
		name := string(component)
		runtime := &fakeRuntime{
			metadata: native.Metadata{Name: name},
			close: func() error {
				closed = append(closed, name)
				return nil
			},
		}
		if name == "b.wasm" {
			runtime.init = func() error { return errors.New("init failed") }
		}
		return runtime, nil
	}
	manager := newWithLoader(cfg, loader)

	err := manager.Start(context.Background(), &proxy.Proxy{})
	require.ErrorContains(t, err, "init failed")
	require.Equal(t, []string{"b.wasm", "a.wasm"}, closed)
}
