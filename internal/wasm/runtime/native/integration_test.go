//go:build wasm_native && cgo

package native

import (
	"os"
	"strings"
	"sync"
	"testing"

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

func TestRuntimeNestedComponentCall(t *testing.T) {
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
