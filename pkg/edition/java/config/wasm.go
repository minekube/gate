package config

import (
	"fmt"
	"strings"
	"time"

	"go.minekube.com/gate/pkg/util/configutil"
)

const maxWasmResourceHandles = 1<<24 - 1

var DefaultWasm = Wasm{
	Enabled:           true,
	Directory:         "plugins",
	MaxComponentBytes: 64 << 20,
	MemoryBytes:       128 << 20,
	TransferBytes:     16 << 20,
	Fuel:              10_000_000,
	ResourceHandles:   65_536,
	TimerLimit:        1_024,
	InitDeadline:      configutil.Duration(5 * time.Second),
	CallbackDeadline:  configutil.Duration(100 * time.Millisecond),
	Plugins:           map[string]WasmPlugin{},
}

// Wasm configures language-neutral WebAssembly Component plugins.
type Wasm struct {
	Enabled           bool                  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Directory         string                `yaml:"directory,omitempty" json:"directory,omitempty"`
	MaxComponentBytes uint64                `yaml:"maxComponentBytes,omitempty" json:"maxComponentBytes,omitempty"`
	MemoryBytes       uint64                `yaml:"memoryBytes,omitempty" json:"memoryBytes,omitempty"`
	TransferBytes     uint64                `yaml:"transferBytes,omitempty" json:"transferBytes,omitempty"`
	Fuel              uint64                `yaml:"fuel,omitempty" json:"fuel,omitempty"`
	ResourceHandles   uint32                `yaml:"resourceHandles,omitempty" json:"resourceHandles,omitempty"`
	TimerLimit        uint32                `yaml:"timerLimit,omitempty" json:"timerLimit,omitempty"`
	InitDeadline      configutil.Duration   `yaml:"initDeadline,omitempty" json:"initDeadline,omitempty"`
	CallbackDeadline  configutil.Duration   `yaml:"callbackDeadline,omitempty" json:"callbackDeadline,omitempty"`
	Plugins           map[string]WasmPlugin `yaml:"plugins,omitempty" json:"plugins,omitempty"`
}

// WasmPlugin contains identity-based component overrides.
type WasmPlugin struct {
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

func (cfg Wasm) Validate() []error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("wasm: "+format, args...))
	}
	if strings.TrimSpace(cfg.Directory) == "" {
		add("directory must not be empty")
	}
	if strings.ContainsRune(cfg.Directory, '\x00') {
		add("directory contains a null byte")
	}
	if cfg.MaxComponentBytes == 0 || cfg.MaxComponentBytes > 1<<30 {
		add("maxComponentBytes must be between 1 and %d", uint64(1<<30))
	}
	if cfg.MemoryBytes < 1<<20 || cfg.MemoryBytes > 4<<30 {
		add("memoryBytes must be between %d and %d", uint64(1<<20), uint64(4<<30))
	}
	if cfg.TransferBytes == 0 || cfg.TransferBytes > cfg.MemoryBytes {
		add("transferBytes must be greater than zero and no larger than memoryBytes")
	}
	if cfg.Fuel == 0 {
		add("fuel must be greater than zero")
	}
	if cfg.ResourceHandles == 0 || cfg.ResourceHandles > maxWasmResourceHandles {
		add("resourceHandles must be between 1 and %d", maxWasmResourceHandles)
	}
	if cfg.TimerLimit == 0 || cfg.TimerLimit > 1<<20 {
		add("timerLimit must be between 1 and %d", 1<<20)
	}
	for name, deadline := range map[string]configutil.Duration{
		"initDeadline":     cfg.InitDeadline,
		"callbackDeadline": cfg.CallbackDeadline,
	} {
		duration := time.Duration(deadline)
		if duration <= 0 || duration > time.Minute {
			add("%s must be greater than zero and no longer than one minute", name)
		}
	}
	for name := range cfg.Plugins {
		if strings.TrimSpace(name) == "" {
			add("plugin identity must not be empty")
		}
	}
	return errs
}
