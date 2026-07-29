// Package abi defines the fixed-width values used at the generated Go/C/Rust
// boundary. These structures describe addresses; they never contain Go
// pointers and must not be dereferenced by this package.
package abi

import "fmt"

const SchemaVersion uint32 = 1

// Handle is a generation-checked typed resource identifier.
type Handle uint64

// Address is an address in the allocator named by the generated operation.
type Address uint64

// BorrowedBuffer is readable only for the duration of the current call.
type BorrowedBuffer struct {
	Pointer Address
	Length  uint64
}

// OwnedBuffer transfers an allocation to the receiver. The generated operation
// identifies the allocator and matching free operation.
type OwnedBuffer struct {
	Pointer  Address
	Length   uint64
	Capacity uint64
}

type OptionHeader struct {
	IsSome uint8
	_      [7]uint8
}

type VariantHeader struct {
	Case uint32
	_    [4]uint8
}

type ResultHeader struct {
	IsError uint32
	_       uint32
	Payload uint64
}

type Lifetime string

const (
	LifetimeCall Lifetime = "call"
)

const BorrowedBufferLifetime = LifetimeCall

type Allocator string

const (
	AllocatorGo   Allocator = "go"
	AllocatorRust Allocator = "rust"
)

type FreeOperation string

const (
	FreeGoBuffer   FreeOperation = "gate_wasm_go_buffer_free"
	FreeRustBuffer FreeOperation = "gate_wasm_rust_buffer_free"
)

func (allocator Allocator) FreeOperation() FreeOperation {
	switch allocator {
	case AllocatorGo:
		return FreeGoBuffer
	case AllocatorRust:
		return FreeRustBuffer
	default:
		return ""
	}
}

func (buffer BorrowedBuffer) Validate() error {
	if buffer.Length != 0 && buffer.Pointer == 0 {
		return fmt.Errorf("non-empty borrowed buffer has a null pointer")
	}
	return nil
}

func (buffer OwnedBuffer) Validate(allocator Allocator) error {
	if allocator.FreeOperation() == "" {
		return fmt.Errorf("owned buffer has unknown allocator %q", allocator)
	}
	if buffer.Length > buffer.Capacity {
		return fmt.Errorf(
			"owned buffer length exceeds capacity: %d > %d",
			buffer.Length,
			buffer.Capacity,
		)
	}
	if buffer.Capacity != 0 && buffer.Pointer == 0 {
		return fmt.Errorf("non-empty owned buffer has a null pointer")
	}
	return nil
}
