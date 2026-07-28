package native

import (
	"errors"
	"time"
)

var (
	ErrUnavailable    = errors.New("wasm native runtime unavailable")
	ErrClosed         = errors.New("wasm runtime closed")
	ErrExpiredReentry = errors.New("wasm reentry token expired")
	ErrFuelExhausted  = errors.New("wasm fuel exhausted")
	ErrDeadline       = errors.New("wasm call deadline exceeded")
	ErrMemoryLimit    = errors.New("wasm memory limit exceeded")
	ErrTransferLimit  = errors.New("wasm transfer limit exceeded")
)

type Sample struct {
	Text   string
	Factor int32
	Tags   []string
}

type Limits struct {
	MemoryBytes   uint64
	TransferBytes uint64
	Fuel          uint64
	Deadline      time.Duration
}

type Host interface {
	ContextCancelled(contextID uint64) (bool, error)
	Transform(proxyID uint64, input Sample) (Sample, error)
	EmitNested(reentry Reentry, proxyID uint64, input string) (string, error)
}

type Reentry interface {
	OnEvent(proxyID uint64, input string) (string, error)
}

type Runtime struct {
	impl runtimeImpl
}

type runtimeImpl interface {
	Init(contextID, proxyID uint64) (Sample, error)
	OnEvent(proxyID uint64, input string) (string, error)
	Allocate(bytes uint64) (uint64, error)
	Spin() error
	Close() error
}

func (r *Runtime) Init(contextID, proxyID uint64) (Sample, error) {
	if r == nil || r.impl == nil {
		return Sample{}, ErrClosed
	}
	return r.impl.Init(contextID, proxyID)
}

func (r *Runtime) OnEvent(proxyID uint64, input string) (string, error) {
	if r == nil || r.impl == nil {
		return "", ErrClosed
	}
	return r.impl.OnEvent(proxyID, input)
}

func (r *Runtime) Allocate(bytes uint64) (uint64, error) {
	if r == nil || r.impl == nil {
		return 0, ErrClosed
	}
	return r.impl.Allocate(bytes)
}

func (r *Runtime) Spin() error {
	if r == nil || r.impl == nil {
		return ErrClosed
	}
	return r.impl.Spin()
}

func (r *Runtime) Close() error {
	if r == nil || r.impl == nil {
		return nil
	}
	return r.impl.Close()
}
