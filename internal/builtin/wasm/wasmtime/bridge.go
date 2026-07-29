//go:build wasm_native && cgo

package native

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo darwin LDFLAGS: ${SRCDIR}/target/release/libgate_wasm_native.a -liconv -lm
#cgo linux LDFLAGS: ${SRCDIR}/target/release/libgate_wasm_native.a -ldl -lpthread -lm
#cgo windows LDFLAGS: -L${SRCDIR}/target/release -l:libgate_wasm_native.a -lws2_32 -luserenv -lbcrypt -lntdll
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"runtime/cgo"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var liveHostHandles atomic.Int64

type nativeRuntime struct {
	mu     sync.Mutex
	ptr    *C.gate_wasm_runtime
	handle cgo.Handle
	closed bool
}

func New(component []byte, host Host, limits Limits) (*Runtime, error) {
	if host == nil {
		return nil, errors.New("wasm host is required")
	}
	handle := cgo.NewHandle(host)
	var cError C.gate_wasm_owned_bytes
	ptr := C.gate_wasm_runtime_new(
		borrowedBytes(component),
		C.uintptr_t(handle),
		C.gate_wasm_limits{
			memory_bytes:   C.uint64_t(limits.MemoryBytes),
			transfer_bytes: C.uint64_t(limits.TransferBytes),
			fuel:           C.uint64_t(limits.Fuel),
			deadline_nanos: deadlineNanos(limits),
		},
		&cError,
	)
	runtime.KeepAlive(component)
	if ptr == nil {
		handle.Delete()
		return nil, takeRustError(cError)
	}
	liveHostHandles.Add(1)
	result := &Runtime{impl: &nativeRuntime{
		ptr:    ptr,
		handle: handle,
	}}
	if binder, ok := host.(interface {
		BindCallbackInvoker(CallbackInvoker) error
	}); ok {
		if err := binder.BindCallbackInvoker(result); err != nil {
			_ = result.Close()
			return nil, fmt.Errorf("bind wasm callback runtime: %w", err)
		}
	}
	return result, nil
}

func deadlineNanos(limits Limits) C.uint64_t {
	if limits.Deadline <= 0 {
		return 0
	}
	return C.uint64_t(limits.Deadline)
}

func nativeRuntimeVersion() string {
	version, err := copySlice(C.gate_wasm_runtime_version())
	if err != nil {
		panic(err)
	}
	return string(version)
}

func (r *nativeRuntime) Metadata() (Metadata, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Metadata{}, ErrClosed
	}
	var output C.gate_wasm_plugin_metadata
	var cError C.gate_wasm_owned_bytes
	status := C.gate_wasm_runtime_metadata(r.ptr, &output, &cError)
	if status != 0 {
		return Metadata{}, takeRustStatus(status, cError)
	}
	name, err := copySlice(output.name)
	if err != nil {
		return Metadata{}, err
	}
	version, err := copySlice(output.version)
	if err != nil {
		return Metadata{}, err
	}
	contractHash, err := copySlice(output.contract_hash)
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{
		Name:            string(name),
		Version:         string(version),
		ContractHash:    string(contractHash),
		GeneratorFormat: uint32(output.generator_format),
	}, nil
}

func (r *nativeRuntime) Init(contextID, proxyID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	var cError C.gate_wasm_owned_bytes
	status := C.gate_wasm_runtime_init(
		r.ptr,
		C.uint64_t(contextID),
		C.uint64_t(proxyID),
		&cError,
	)
	if status != 0 {
		return takeRustStatus(status, cError)
	}
	return nil
}

func (r *nativeRuntime) SetDeadline(deadline time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	var cError C.gate_wasm_owned_bytes
	status := C.gate_wasm_runtime_set_deadline(
		r.ptr,
		C.uint64_t(max(deadline, 0)),
		&cError,
	)
	if status != 0 {
		return takeRustStatus(status, cError)
	}
	return nil
}

func (r *nativeRuntime) InvokeCallback(
	callbackTypeID uint32,
	guestID uint64,
	input []byte,
) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrClosed
	}
	var output C.gate_wasm_owned_bytes
	var cError C.gate_wasm_owned_bytes
	status := C.gate_wasm_runtime_invoke_callback(
		r.ptr,
		C.uint32_t(callbackTypeID),
		C.uint64_t(guestID),
		borrowedBytes(input),
		&output,
		&cError,
	)
	runtime.KeepAlive(input)
	if status != 0 {
		return nil, takeRustStatus(status, cError)
	}
	return takeRustBytes(output)
}

func (r *nativeRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	C.gate_wasm_runtime_free(r.ptr)
	r.ptr = nil
	r.handle.Delete()
	liveHostHandles.Add(-1)
	return nil
}

func borrowedBytes(value []byte) C.gate_wasm_slice {
	var ptr *C.uint8_t
	if len(value) != 0 {
		ptr = (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(value)))
	}
	return C.gate_wasm_slice{ptr: ptr, len: C.size_t(len(value))}
}

func takeRustBytes(value C.gate_wasm_owned_bytes) ([]byte, error) {
	defer C.gate_wasm_owned_bytes_free(value)
	if value.len == 0 {
		return nil, nil
	}
	if value.ptr == nil {
		return nil, errors.New("Rust returned a non-empty null buffer")
	}
	if uint64(value.len) > math.MaxInt {
		return nil, fmt.Errorf("Rust buffer length %d exceeds Go int", uint64(value.len))
	}
	return C.GoBytes(unsafe.Pointer(value.ptr), C.int(value.len)), nil
}

func takeRustError(value C.gate_wasm_owned_bytes) error {
	bytes, err := takeRustBytes(value)
	if err != nil {
		return err
	}
	if len(bytes) == 0 {
		return errors.New("wasm runtime failed without an error message")
	}
	return errors.New(string(bytes))
}

func takeRustStatus(status C.int32_t, value C.gate_wasm_owned_bytes) error {
	detail := takeRustError(value)
	var kind error
	switch status {
	case C.GATE_WASM_STATUS_FUEL:
		kind = ErrFuelExhausted
	case C.GATE_WASM_STATUS_DEADLINE:
		kind = ErrDeadline
	case C.GATE_WASM_STATUS_MEMORY:
		kind = ErrMemoryLimit
	case C.GATE_WASM_STATUS_TRANSFER:
		kind = ErrTransferLimit
	default:
		return detail
	}
	return errors.Join(kind, detail)
}
