//go:build wasm_native && cgo

package native

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo darwin LDFLAGS: ${SRCDIR}/target/release/libgate_wasm_native.a -liconv -lm
#cgo linux LDFLAGS: ${SRCDIR}/target/release/libgate_wasm_native.a -ldl -lpthread -lm
#cgo windows LDFLAGS: ${SRCDIR}/target/release/gate_wasm_native.lib -lws2_32 -luserenv -lbcrypt -lntdll
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
	"unsafe"
)

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
	return &Runtime{impl: &nativeRuntime{
		ptr:    ptr,
		handle: handle,
	}}, nil
}

func deadlineNanos(limits Limits) C.uint64_t {
	if limits.Deadline <= 0 {
		return 0
	}
	return C.uint64_t(limits.Deadline)
}

func (r *nativeRuntime) Init(contextID, proxyID uint64) (Sample, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Sample{}, ErrClosed
	}
	var output C.gate_wasm_owned_sample
	var cError C.gate_wasm_owned_bytes
	status := C.gate_wasm_runtime_init(
		r.ptr,
		C.uint64_t(contextID),
		C.uint64_t(proxyID),
		&output,
		&cError,
	)
	if status != 0 {
		return Sample{}, takeRustError(cError)
	}
	return takeRustSample(output)
}

func (r *nativeRuntime) OnEvent(proxyID uint64, input string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return "", ErrClosed
	}
	var output C.gate_wasm_owned_bytes
	var cError C.gate_wasm_owned_bytes
	status := C.gate_wasm_runtime_on_event(
		r.ptr,
		C.uint64_t(proxyID),
		borrowedString(input),
		&output,
		&cError,
	)
	runtime.KeepAlive(input)
	if status != 0 {
		return "", takeRustError(cError)
	}
	bytes, err := takeRustBytes(output)
	return string(bytes), err
}

func (r *nativeRuntime) Allocate(bytes uint64) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrClosed
	}
	var output C.uint64_t
	var cError C.gate_wasm_owned_bytes
	status := C.gate_wasm_runtime_allocate(
		r.ptr,
		C.uint64_t(bytes),
		&output,
		&cError,
	)
	if status != 0 {
		return 0, takeRustError(cError)
	}
	return uint64(output), nil
}

func (r *nativeRuntime) Spin() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	var cError C.gate_wasm_owned_bytes
	if C.gate_wasm_runtime_spin(r.ptr, &cError) != 0 {
		return takeRustError(cError)
	}
	return nil
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
	return nil
}

func borrowedBytes(value []byte) C.gate_wasm_slice {
	var ptr *C.uint8_t
	if len(value) != 0 {
		ptr = (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(value)))
	}
	return C.gate_wasm_slice{ptr: ptr, len: C.size_t(len(value))}
}

func borrowedString(value string) C.gate_wasm_slice {
	var ptr *C.uint8_t
	if len(value) != 0 {
		ptr = (*C.uint8_t)(unsafe.Pointer(unsafe.StringData(value)))
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

func takeRustSample(value C.gate_wasm_owned_sample) (Sample, error) {
	defer C.gate_wasm_owned_sample_free(value)
	text, err := copyRustBytes(value.text)
	if err != nil {
		return Sample{}, err
	}
	tagValues, err := rustOwnedBytesSlice(value.tags)
	if err != nil {
		return Sample{}, err
	}
	tags := make([]string, len(tagValues))
	for i, tag := range tagValues {
		bytes, err := copyRustBytes(tag)
		if err != nil {
			return Sample{}, err
		}
		tags[i] = string(bytes)
	}
	return Sample{
		Text:   string(text),
		Factor: int32(value.factor),
		Tags:   tags,
	}, nil
}

func copyRustBytes(value C.gate_wasm_owned_bytes) ([]byte, error) {
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

func rustOwnedBytesSlice(
	value C.gate_wasm_owned_bytes_list,
) ([]C.gate_wasm_owned_bytes, error) {
	if value.len == 0 {
		return nil, nil
	}
	if value.ptr == nil {
		return nil, errors.New("Rust returned a non-empty null buffer list")
	}
	if uint64(value.len) > math.MaxInt {
		return nil, fmt.Errorf("Rust buffer-list length %d exceeds Go int", uint64(value.len))
	}
	return unsafe.Slice(value.ptr, int(value.len)), nil
}
