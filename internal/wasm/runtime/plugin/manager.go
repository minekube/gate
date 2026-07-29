// Package plugin discovers and owns language-neutral WebAssembly Component
// plugins for one Gate proxy.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"go.minekube.com/gate/internal/wasm/runtime/gatehost"
	"go.minekube.com/gate/internal/wasm/runtime/native"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

type componentRuntime interface {
	Metadata() (native.Metadata, error)
	Init(contextID, proxyID uint64) error
	SetDeadline(deadline time.Duration) error
	InvokeCallback(callbackTypeID uint32, guestID uint64, input []byte) ([]byte, error)
	Close() error
}

type loader func(
	component []byte,
	host *gatehost.Host,
	limits native.Limits,
) (componentRuntime, error)

type loadedPlugin struct {
	path     string
	metadata native.Metadata
	runtime  componentRuntime
	host     *gatehost.Host
}

type Manager struct {
	mu      sync.Mutex
	config  config.Wasm
	loader  loader
	plugins []*loadedPlugin
	started bool
	closed  bool
}

func New(cfg config.Wasm) *Manager {
	return newWithLoader(cfg, func(
		component []byte,
		host *gatehost.Host,
		limits native.Limits,
	) (componentRuntime, error) {
		return native.New(component, host, limits)
	})
}

func newWithLoader(cfg config.Wasm, load loader) *Manager {
	return &Manager{config: cfg, loader: load}
}

func (manager *Manager) Start(ctx context.Context, gateProxy *proxy.Proxy) (err error) {
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return errors.New("wasm plugin manager is closed")
	}
	if manager.started {
		manager.mu.Unlock()
		return errors.New("wasm plugin manager is already started")
	}
	manager.started = true
	manager.mu.Unlock()

	if !manager.config.Enabled {
		return nil
	}
	if ctx == nil || gateProxy == nil {
		return errors.New("wasm plugin manager requires context and proxy")
	}
	paths, err := componentPaths(manager.config.Directory)
	if err != nil {
		return err
	}
	identities := make(map[string]string, len(paths))
	for _, path := range paths {
		component, err := readComponent(path, manager.config.MaxComponentBytes)
		if err != nil {
			manager.closeAfterFailure()
			return err
		}
		host, err := gatehost.New(
			filepath.Base(path),
			ctx,
			gateProxy,
			manager.config.ResourceHandles,
		)
		if err != nil {
			manager.closeAfterFailure()
			return fmt.Errorf("prepare wasm plugin %s: %w", path, err)
		}
		if err := host.SetTimerLimit(manager.config.TimerLimit); err != nil {
			_ = host.Close()
			manager.closeAfterFailure()
			return fmt.Errorf("configure wasm plugin %s: %w", path, err)
		}
		runtime, err := manager.loader(component, host, manager.limits())
		if err != nil {
			_ = host.Close()
			manager.closeAfterFailure()
			return fmt.Errorf("load wasm plugin %s: %w", path, err)
		}
		var serialized *executor
		serialized = newExecutor(runtime, func(error) {
			host.StopRegistrations()
			go func() {
				_ = serialized.Close()
				_ = host.Close()
			}()
		})
		if err := host.ReplaceCallbackInvoker(serialized); err != nil {
			_ = serialized.Close()
			_ = host.Close()
			manager.closeAfterFailure()
			return fmt.Errorf("bind wasm plugin executor %s: %w", path, err)
		}
		plugin := &loadedPlugin{path: path, runtime: serialized, host: host}
		metadata, err := serialized.Metadata()
		if err != nil {
			manager.closePlugin(plugin)
			manager.closeAfterFailure()
			return fmt.Errorf("read wasm plugin metadata %s: %w", path, err)
		}
		plugin.metadata = metadata
		if previous, duplicate := identities[metadata.Name]; duplicate {
			manager.closePlugin(plugin)
			manager.closeAfterFailure()
			return fmt.Errorf(
				"duplicate wasm plugin identity %q in %s and %s",
				metadata.Name,
				previous,
				path,
			)
		}
		identities[metadata.Name] = path
		if override, configured := manager.config.Plugins[metadata.Name]; configured && override.Disabled {
			manager.closePlugin(plugin)
			continue
		}
		if err := serialized.Init(host.ContextHandle(), host.ProxyHandle()); err != nil {
			manager.closePlugin(plugin)
			manager.closeAfterFailure()
			return fmt.Errorf(
				"initialize wasm plugin %q from %s: %w",
				metadata.Name,
				path,
				err,
			)
		}
		if err := serialized.SetDeadline(time.Duration(manager.config.CallbackDeadline)); err != nil {
			manager.closePlugin(plugin)
			manager.closeAfterFailure()
			return fmt.Errorf(
				"configure wasm plugin %q callback deadline: %w",
				metadata.Name,
				err,
			)
		}
		manager.mu.Lock()
		manager.plugins = append(manager.plugins, plugin)
		manager.mu.Unlock()
	}
	return nil
}

func (manager *Manager) limits() native.Limits {
	return native.Limits{
		MemoryBytes:   manager.config.MemoryBytes,
		TransferBytes: manager.config.TransferBytes,
		Fuel:          manager.config.Fuel,
		Deadline:      time.Duration(manager.config.InitDeadline),
	}
}

func (manager *Manager) Names() []string {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	names := make([]string, len(manager.plugins))
	for index, plugin := range manager.plugins {
		names[index] = plugin.metadata.Name
	}
	return names
}

func (manager *Manager) Close() error {
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return nil
	}
	manager.closed = true
	plugins := manager.plugins
	manager.plugins = nil
	manager.mu.Unlock()
	return closePlugins(plugins)
}

func (manager *Manager) closeAfterFailure() {
	manager.mu.Lock()
	plugins := manager.plugins
	manager.plugins = nil
	manager.closed = true
	manager.mu.Unlock()
	_ = closePlugins(plugins)
}

func (manager *Manager) closePlugin(plugin *loadedPlugin) error {
	if plugin == nil {
		return nil
	}
	plugin.host.StopRegistrations()
	return errors.Join(plugin.runtime.Close(), plugin.host.Close())
}

func closePlugins(plugins []*loadedPlugin) error {
	var errs []error
	for index := len(plugins) - 1; index >= 0; index-- {
		plugin := plugins[index]
		plugin.host.StopRegistrations()
		if err := plugin.runtime.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close wasm plugin %q: %w", plugin.metadata.Name, err))
		}
		if err := plugin.host.Close(); err != nil {
			errs = append(errs, fmt.Errorf("release wasm plugin %q: %w", plugin.metadata.Name, err))
		}
	}
	return errors.Join(errs...)
}

func componentPaths(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read wasm plugin directory %s: %w", directory, err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.Type().IsRegular() &&
			strings.EqualFold(filepath.Ext(entry.Name()), ".wasm") {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	slices.Sort(paths)
	return paths, nil
}

func readComponent(path string, maximum uint64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open wasm plugin %s: %w", path, err)
	}
	defer file.Close()
	limit := maximum + 1
	if limit == 0 {
		return nil, fmt.Errorf("wasm plugin size limit overflowed")
	}
	component, err := io.ReadAll(io.LimitReader(file, int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("read wasm plugin %s: %w", path, err)
	}
	if uint64(len(component)) > maximum {
		return nil, fmt.Errorf(
			"wasm plugin %s is %d bytes, limit is %d",
			path,
			len(component),
			maximum,
		)
	}
	return component, nil
}
