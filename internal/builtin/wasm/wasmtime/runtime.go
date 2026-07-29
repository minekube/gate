package native

import (
	"errors"
	"time"
)

var (
	ErrUnavailable        = errors.New("wasm native runtime unavailable")
	ErrClosed             = errors.New("wasm runtime closed")
	ErrExpiredReentry     = errors.New("wasm reentry token expired")
	ErrFuelExhausted      = errors.New("wasm fuel exhausted")
	ErrDeadline           = errors.New("wasm call deadline exceeded")
	ErrMemoryLimit        = errors.New("wasm memory limit exceeded")
	ErrTransferLimit      = errors.New("wasm transfer limit exceeded")
	ErrWrongReentryThread = errors.New("wasm reentry token used from another OS thread")
)

type Limits struct {
	MemoryBytes   uint64
	TransferBytes uint64
	Fuel          uint64
	Deadline      time.Duration
}

type Metadata struct {
	Name            string
	Version         string
	ContractHash    string
	GeneratorFormat uint32
}

type Host interface{}

type CallbackInvoker interface {
	InvokeCallback(callbackTypeID uint32, guestID uint64, input []byte) ([]byte, error)
}

type Runtime struct {
	impl runtimeImpl
}

type runtimeImpl interface {
	Metadata() (Metadata, error)
	Init(contextID, proxyID uint64) error
	SetDeadline(deadline time.Duration) error
	InvokeCallback(callbackTypeID uint32, guestID uint64, input []byte) ([]byte, error)
	Close() error
}

func (r *Runtime) Metadata() (Metadata, error) {
	if r == nil || r.impl == nil {
		return Metadata{}, ErrClosed
	}
	return r.impl.Metadata()
}

func (r *Runtime) Init(contextID, proxyID uint64) error {
	if r == nil || r.impl == nil {
		return ErrClosed
	}
	return r.impl.Init(contextID, proxyID)
}

func (r *Runtime) SetDeadline(deadline time.Duration) error {
	if r == nil || r.impl == nil {
		return ErrClosed
	}
	return r.impl.SetDeadline(deadline)
}

func (r *Runtime) InvokeCallback(
	callbackTypeID uint32,
	guestID uint64,
	input []byte,
) ([]byte, error) {
	if r == nil || r.impl == nil {
		return nil, ErrClosed
	}
	return r.impl.InvokeCallback(callbackTypeID, guestID, input)
}

func (r *Runtime) Close() error {
	if r == nil || r.impl == nil {
		return nil
	}
	return r.impl.Close()
}
