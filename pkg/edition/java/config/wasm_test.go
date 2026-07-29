package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/util/configutil"
)

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

func TestWasmConfigValidationRejectsDangerousLimits(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*Wasm){
		"directory": func(cfg *Wasm) { cfg.Directory = "" },
		"memory":    func(cfg *Wasm) { cfg.MemoryBytes = 0 },
		"transfer": func(cfg *Wasm) {
			cfg.TransferBytes = cfg.MemoryBytes + 1
		},
		"fuel":    func(cfg *Wasm) { cfg.Fuel = 0 },
		"handles": func(cfg *Wasm) { cfg.ResourceHandles = 1 << 24 },
		"timers":  func(cfg *Wasm) { cfg.TimerLimit = 0 },
		"init":    func(cfg *Wasm) { cfg.InitDeadline = 0 },
		"callback": func(cfg *Wasm) {
			cfg.CallbackDeadline = configutil.Duration(2 * time.Minute)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultWasm
			mutate(&cfg)
			require.NotEmpty(t, cfg.Validate())
		})
	}
}

func TestConfigValidationIncludesWasm(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig
	cfg.Wasm.Directory = ""
	_, errs := cfg.Validate()
	require.ErrorContains(t, errs[len(errs)-1], "wasm")
}
