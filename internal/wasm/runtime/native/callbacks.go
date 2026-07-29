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

type nativeReentry struct {
	mu       sync.Mutex
	ptr      *C.gate_wasm_reentry
	threadID C.uintptr_t
	active   bool
}

func (r *nativeReentry) OnEvent(proxyID uint64, input string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active || r.ptr == nil {
		return "", ErrExpiredReentry
	}
	if C.gate_wasm_current_thread_id() != r.threadID {
		return "", errWrongReentryThread
	}
	var output C.gate_wasm_owned_bytes
	var cError C.gate_wasm_owned_bytes
	status := C.gate_wasm_reentry_call(
		r.ptr,
		C.uint64_t(proxyID),
		borrowedString(input),
		&output,
		&cError,
	)
	runtime.KeepAlive(input)
	if status != 0 {
		return "", takeRustStatus(status, cError)
	}
	bytes, err := takeRustBytes(output)
	return string(bytes), err
}

func (r *nativeReentry) expire() {
	r.mu.Lock()
	r.active = false
	r.ptr = nil
	r.mu.Unlock()
}

//export gate_wasm_go_context_cancelled
func gate_wasm_go_context_cancelled(
	host C.uintptr_t,
	contextID C.uint64_t,
	cancelled *C.uint8_t,
	errorOutput *C.gate_wasm_owned_bytes,
) (status C.int32_t) {
	clearOwnedBytes(errorOutput)
	defer recoverCallback(&status, errorOutput)
	if cancelled == nil {
		return callbackError(errorOutput, errors.New("cancelled output is null"))
	}
	value, err := goHost(host).ContextCancelled(uint64(contextID))
	if err != nil {
		return callbackError(errorOutput, err)
	}
	if value {
		*cancelled = 1
	} else {
		*cancelled = 0
	}
	return 0
}

//export gate_wasm_go_transform
func gate_wasm_go_transform(
	host C.uintptr_t,
	proxyID C.uint64_t,
	input C.gate_wasm_sample_view,
	output *C.gate_wasm_owned_sample,
	errorOutput *C.gate_wasm_owned_bytes,
) (status C.int32_t) {
	clearOwnedBytes(errorOutput)
	if output != nil {
		*output = C.gate_wasm_owned_sample{}
	}
	defer recoverCallback(&status, errorOutput)
	if output == nil {
		return callbackError(errorOutput, errors.New("sample output is null"))
	}
	sample, err := copySampleView(input)
	if err != nil {
		return callbackError(errorOutput, err)
	}
	result, err := goHost(host).Transform(uint64(proxyID), sample)
	if err != nil {
		return callbackError(errorOutput, err)
	}
	if err := setCSample(output, result); err != nil {
		return callbackError(errorOutput, err)
	}
	return 0
}

//export gate_wasm_go_emit_nested
func gate_wasm_go_emit_nested(
	host C.uintptr_t,
	reentryPointer *C.gate_wasm_reentry,
	proxyID C.uint64_t,
	input C.gate_wasm_slice,
	output *C.gate_wasm_owned_bytes,
	errorOutput *C.gate_wasm_owned_bytes,
) (status C.int32_t) {
	clearOwnedBytes(output)
	clearOwnedBytes(errorOutput)
	defer recoverCallback(&status, errorOutput)
	if reentryPointer == nil {
		return callbackError(errorOutput, errors.New("reentry pointer is null"))
	}
	if output == nil {
		return callbackError(errorOutput, errors.New("nested output is null"))
	}
	inputBytes, err := copySlice(input)
	if err != nil {
		return callbackError(errorOutput, err)
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	reentry := &nativeReentry{
		ptr:      reentryPointer,
		threadID: C.gate_wasm_current_thread_id(),
		active:   true,
	}
	defer reentry.expire()

	result, err := goHost(host).EmitNested(reentry, uint64(proxyID), string(inputBytes))
	if err != nil {
		return callbackError(errorOutput, err)
	}
	if err := setCBytes(output, []byte(result)); err != nil {
		return callbackError(errorOutput, err)
	}
	return 0
}

type dispatchHost interface {
	Invoke(operationID uint32, input []byte) ([]byte, error)
	DropResource(handle uint64) error
}

//export gate_wasm_go_invoke
func gate_wasm_go_invoke(
	host C.uintptr_t,
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
	inputBytes, err := copySlice(input)
	if err != nil {
		return callbackError(errorOutput, err)
	}
	hostValue, ok := goHost(host).(dispatchHost)
	if !ok {
		return callbackError(errorOutput, errors.New("Go host does not provide Gate API dispatch"))
	}
	result, err := hostValue.Invoke(uint32(operationID), inputBytes)
	if err != nil {
		return callbackError(errorOutput, err)
	}
	if err := setCBytes(output, result); err != nil {
		return callbackError(errorOutput, err)
	}
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

func freeCBytes(value *C.gate_wasm_owned_bytes) {
	if value != nil && value.ptr != nil {
		C.free(unsafe.Pointer(value.ptr))
		*value = C.gate_wasm_owned_bytes{}
	}
}

func setCSample(output *C.gate_wasm_owned_sample, sample Sample) (err error) {
	if err := setCBytes(&output.text, []byte(sample.Text)); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			freeCBytes(&output.text)
			freeCByteList(&output.tags)
		}
	}()
	output.factor = C.int32_t(sample.Factor)
	if len(sample.Tags) == 0 {
		return nil
	}
	if len(sample.Tags) > math.MaxInt/int(unsafe.Sizeof(C.gate_wasm_owned_bytes{})) {
		return errors.New("too many sample tags")
	}
	size := C.size_t(len(sample.Tags)) * C.size_t(unsafe.Sizeof(C.gate_wasm_owned_bytes{}))
	pointer := C.malloc(size)
	if pointer == nil {
		return errors.New("C tag-list allocation failed")
	}
	C.gate_wasm_owned_bytes_list_set(
		&output.tags,
		(*C.gate_wasm_owned_bytes)(pointer),
		C.size_t(len(sample.Tags)),
		C.size_t(len(sample.Tags)),
	)
	tags := unsafe.Slice(output.tags.ptr, len(sample.Tags))
	for index, tag := range sample.Tags {
		if err := setCBytes(&tags[index], []byte(tag)); err != nil {
			return err
		}
	}
	return nil
}

func freeCByteList(value *C.gate_wasm_owned_bytes_list) {
	if value == nil || value.ptr == nil {
		return
	}
	for index := range unsafe.Slice(value.ptr, int(value.len)) {
		freeCBytes(&unsafe.Slice(value.ptr, int(value.len))[index])
	}
	C.free(unsafe.Pointer(value.ptr))
	*value = C.gate_wasm_owned_bytes_list{}
}

func copySampleView(input C.gate_wasm_sample_view) (Sample, error) {
	text, err := copySlice(input.text)
	if err != nil {
		return Sample{}, err
	}
	tags, err := copySliceList(input.tags)
	if err != nil {
		return Sample{}, err
	}
	return Sample{
		Text:   string(text),
		Factor: int32(input.factor),
		Tags:   tags,
	}, nil
}

func copySliceList(value C.gate_wasm_slice_list) ([]string, error) {
	if value.len == 0 {
		return nil, nil
	}
	if value.ptr == nil {
		return nil, errors.New("non-empty C slice list has a null pointer")
	}
	if uint64(value.len) > math.MaxInt {
		return nil, errors.New("C slice list is too large")
	}
	items := unsafe.Slice(value.ptr, int(value.len))
	result := make([]string, len(items))
	for index, item := range items {
		bytes, err := copySlice(item)
		if err != nil {
			return nil, err
		}
		result[index] = string(bytes)
	}
	return result, nil
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
