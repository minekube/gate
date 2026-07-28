//go:build wasm_native && cgo

package native

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type integrationHost struct {
	mu       sync.Mutex
	calls    []string
	retained Reentry
}

func (h *integrationHost) ContextCancelled(contextID uint64) (bool, error) {
	if contextID != 1 {
		panic("unexpected context ID")
	}
	return false, nil
}

func (h *integrationHost) Transform(proxyID uint64, input Sample) (Sample, error) {
	if proxyID != 2 {
		panic("unexpected proxy ID")
	}
	if event, ok := strings.CutPrefix(input.Text, "event:"); ok {
		h.mu.Lock()
		h.calls = append(h.calls, "guest:"+event)
		h.mu.Unlock()
		return input, nil
	}
	input.Text = "host:" + input.Text
	input.Factor *= 3
	input.Tags = append(input.Tags, "host")
	return input, nil
}

func (h *integrationHost) EmitNested(
	reentry Reentry,
	proxyID uint64,
	input string,
) (string, error) {
	h.mu.Lock()
	h.calls = append(h.calls, "host:emit-nested")
	h.retained = reentry
	h.mu.Unlock()
	result, err := reentry.OnEvent(proxyID, input)
	h.mu.Lock()
	h.calls = append(h.calls, "host:return-nested")
	h.mu.Unlock()
	return result, err
}

func TestRuntime_NestedComponentCall(t *testing.T) {
	component, err := os.ReadFile("artifacts/gate_wasm_spike.component.wasm")
	require.NoError(t, err)
	host := &integrationHost{}
	runtime, err := New(component, host, Limits{})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runtime.Close())
	})

	initResult, err := runtime.Init(1, 2)
	require.NoError(t, err)
	require.Equal(t, Sample{
		Text:   "host:init",
		Factor: 6,
		Tags:   []string{"guest", "component", "host"},
	}, initResult)

	eventResult, err := runtime.OnEvent(2, "outer")
	require.NoError(t, err)
	require.Equal(t, "outer:guest:inner", eventResult)
	require.Equal(t, []string{
		"guest:outer",
		"host:emit-nested",
		"guest:inner",
		"host:return-nested",
	}, host.calls)

	_, err = host.retained.OnEvent(2, "expired")
	require.ErrorIs(t, err, ErrExpiredReentry)
	require.NoError(t, runtime.Close())
	require.NoError(t, runtime.Close())
}

func TestRuntime_LimitsFuel(t *testing.T) {
	runtime := newIntegrationRuntime(t, Limits{
		Fuel:     1_000,
		Deadline: time.Second,
	})
	defer func() { require.NoError(t, runtime.Close()) }()

	started := time.Now()
	err := runtime.Spin()

	require.ErrorIs(t, err, ErrFuelExhausted)
	require.Less(t, time.Since(started), time.Second)
}

func TestRuntime_LimitsDeadline(t *testing.T) {
	runtime := newIntegrationRuntime(t, Limits{
		Fuel:     ^uint64(0),
		Deadline: 25 * time.Millisecond,
	})
	defer func() { require.NoError(t, runtime.Close()) }()

	started := time.Now()
	err := runtime.Spin()

	require.ErrorIs(t, err, ErrDeadline)
	require.Less(t, time.Since(started), time.Second)
}

func TestRuntime_LimitsMemory(t *testing.T) {
	runtime := newIntegrationRuntime(t, Limits{
		MemoryBytes: 2 << 20,
		Deadline:    time.Second,
	})
	defer func() { require.NoError(t, runtime.Close()) }()

	_, err := runtime.Allocate(8 << 20)

	require.ErrorIs(t, err, ErrMemoryLimit)
}

func TestRuntime_LimitsTransfer(t *testing.T) {
	runtime := newIntegrationRuntime(t, Limits{
		TransferBytes: 64,
		Deadline:      time.Second,
	})
	defer func() { require.NoError(t, runtime.Close()) }()

	_, err := runtime.OnEvent(2, "large")

	require.ErrorIs(t, err, ErrTransferLimit)

	_, err = runtime.OnEvent(2, strings.Repeat("x", 65))
	require.ErrorIs(t, err, ErrTransferLimit)
}

func TestRuntime_CloseAfterFailedCall(t *testing.T) {
	before := liveHostHandles.Load()
	runtime := newIntegrationRuntime(t, Limits{
		Fuel:     1_000,
		Deadline: 25 * time.Millisecond,
	})
	require.Equal(t, before+1, liveHostHandles.Load())

	require.Error(t, runtime.Spin())
	require.NoError(t, runtime.Close())
	require.NoError(t, runtime.Close())
	require.Equal(t, before, liveHostHandles.Load())
	require.ErrorIs(t, runtime.Spin(), ErrClosed)
}

func TestNativeLibraryIsLinked(t *testing.T) {
	require.Equal(t, "wasmtime-47.0.2", nativeRuntimeVersion())
}

func newIntegrationRuntime(t *testing.T, limits Limits) *Runtime {
	t.Helper()
	component, err := os.ReadFile("artifacts/gate_wasm_spike.component.wasm")
	require.NoError(t, err)
	runtime, err := New(component, &integrationHost{}, limits)
	require.NoError(t, err)
	return runtime
}
