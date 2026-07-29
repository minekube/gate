//go:build wasm_native && cgo

package native

/*
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

var errWrongReentryThread = errors.New("wasm reentry token used from another OS thread")

type nativeCallbackReentry struct {
	mu       sync.Mutex
	ptr      *C.gate_wasm_callback_reentry
	threadID C.uintptr_t
	active   bool
}

func (reentry *nativeCallbackReentry) InvokeCallback(
	callbackTypeID uint32,
	guestID uint64,
	input []byte,
) ([]byte, error) {
	reentry.mu.Lock()
	defer reentry.mu.Unlock()
	if !reentry.active || reentry.ptr == nil {
		return nil, ErrExpiredReentry
	}
	if C.gate_wasm_current_thread_id() != reentry.threadID {
		return nil, errWrongReentryThread
	}
	var output C.gate_wasm_owned_bytes
	var cError C.gate_wasm_owned_bytes
	status := C.gate_wasm_callback_reentry_invoke(
		reentry.ptr,
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

func (reentry *nativeCallbackReentry) expire() {
	reentry.mu.Lock()
	reentry.active = false
	reentry.ptr = nil
	reentry.mu.Unlock()
}

type dispatchHost interface {
	Invoke(operationID uint32, input []byte) ([]byte, error)
	InvokeWithReentry(
		reentry CallbackInvoker,
		operationID uint32,
		input []byte,
	) ([]byte, error)
	RegisterCallback(callbackTypeID uint32, guestID uint64) (uint64, error)
	DropResource(handle uint64) error
}

//export gate_wasm_go_invoke
func gate_wasm_go_invoke(
	host C.uintptr_t,
	reentryPointer *C.gate_wasm_callback_reentry,
	operationID C.uint32_t,
	input C.gate_wasm_slice,
	output *C.gate_wasm_owned_bytes,
	errorOutput *C.gate_wasm_owned_bytes,
) (status C.int32_t) {
	clearOwnedBytes(output)
	clearOwnedBytes(errorOutput)
	defer recoverCallback(&status, errorOutput)
	if output == nil {
		return callbackError(errorOutput, errors.New("dispatch output is null"))
	}
	if reentryPointer == nil {
		return callbackError(errorOutput, errors.New("callback reentry pointer is null"))
	}
	inputBytes, err := copySlice(input)
	if err != nil {
		return callbackError(errorOutput, err)
	}
	hostValue, ok := goHost(host).(dispatchHost)
	if !ok {
		return callbackError(errorOutput, errors.New("Go host does not provide Gate API dispatch"))
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	reentry := &nativeCallbackReentry{
		ptr:      reentryPointer,
		threadID: C.gate_wasm_current_thread_id(),
		active:   true,
	}
	defer reentry.expire()

	result, err := hostValue.InvokeWithReentry(
		reentry,
		uint32(operationID),
		inputBytes,
	)
	if err != nil {
		return callbackError(errorOutput, err)
	}
	if err := setCBytes(output, result); err != nil {
		return callbackError(errorOutput, err)
	}
	return 0
}

//export gate_wasm_go_register_callback
func gate_wasm_go_register_callback(
	host C.uintptr_t,
	callbackTypeID C.uint32_t,
	guestID C.uint64_t,
	handle *C.uint64_t,
	errorOutput *C.gate_wasm_owned_bytes,
) (status C.int32_t) {
	clearOwnedBytes(errorOutput)
	defer recoverCallback(&status, errorOutput)
	if handle == nil {
		return callbackError(errorOutput, errors.New("callback handle output is null"))
	}
	hostValue, ok := goHost(host).(dispatchHost)
	if !ok {
		return callbackError(errorOutput, errors.New("Go host does not provide callback dispatch"))
	}
	value, err := hostValue.RegisterCallback(
		uint32(callbackTypeID),
		uint64(guestID),
	)
	if err != nil {
		return callbackError(errorOutput, err)
	}
	*handle = C.uint64_t(value)
	return 0
}

//export gate_wasm_go_drop_resource
func gate_wasm_go_drop_resource(
	host C.uintptr_t,
	handle C.uint64_t,
	errorOutput *C.gate_wasm_owned_bytes,
) (status C.int32_t) {
	clearOwnedBytes(errorOutput)
	defer recoverCallback(&status, errorOutput)
	hostValue, ok := goHost(host).(dispatchHost)
	if !ok {
		return callbackError(errorOutput, errors.New("Go host does not provide resource dispatch"))
	}
	if err := hostValue.DropResource(uint64(handle)); err != nil {
		return callbackError(errorOutput, err)
	}
	return 0
}

func goHost(handle C.uintptr_t) Host {
	return cgo.Handle(handle).Value().(Host)
}

func recoverCallback(status *C.int32_t, errorOutput *C.gate_wasm_owned_bytes) {
	if recovered := recover(); recovered != nil {
		*status = callbackError(errorOutput, fmt.Errorf("Go host callback panicked: %v", recovered))
	}
}

func callbackError(output *C.gate_wasm_owned_bytes, err error) C.int32_t {
	clearOwnedBytes(output)
	_ = setCBytes(output, []byte(err.Error()))
	return 1
}

func clearOwnedBytes(output *C.gate_wasm_owned_bytes) {
	if output != nil {
		*output = C.gate_wasm_owned_bytes{}
	}
}

func setCBytes(output *C.gate_wasm_owned_bytes, value []byte) error {
	if output == nil {
		return errors.New("C byte output is null")
	}
	if len(value) == 0 {
		*output = C.gate_wasm_owned_bytes{}
		return nil
	}
	pointer := C.malloc(C.size_t(len(value)))
	if pointer == nil {
		return errors.New("C allocation failed")
	}
	copy(unsafe.Slice((*byte)(pointer), len(value)), value)
	C.gate_wasm_owned_bytes_set(
		output,
		(*C.uint8_t)(pointer),
		C.size_t(len(value)),
		C.size_t(len(value)),
	)
	return nil
}

func copySlice(value C.gate_wasm_slice) ([]byte, error) {
	if value.len == 0 {
		return nil, nil
	}
	if value.ptr == nil {
		return nil, errors.New("non-empty C slice has a null pointer")
	}
	if uint64(value.len) > math.MaxInt {
		return nil, errors.New("C slice is too large")
	}
	return C.GoBytes(unsafe.Pointer(value.ptr), C.int(value.len)), nil
}
