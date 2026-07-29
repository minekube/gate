package resources

import (
	"fmt"

	"go.minekube.com/gate/internal/builtin/wasm/runtime/abi"
)

// Handle contains a 24-bit table owner, 16-bit generation, and 24-bit
// one-based slot. It never contains a Go pointer.
type Handle = abi.Handle

type HandleParts struct {
	Owner      uint32
	Generation uint16
	Slot       uint32
}

const (
	handlePartMask uint64 = 1<<24 - 1
	maxOwner              = uint32(handlePartMask)
	maxSlot               = uint32(handlePartMask)
)

func encodeHandle(owner uint32, generation uint16, slot uint32) (Handle, error) {
	if owner == 0 || owner > maxOwner ||
		generation == 0 ||
		slot == 0 || slot > maxSlot {
		return 0, &Error{Kind: ErrorInvalidHandle}
	}
	return Handle(
		uint64(owner)<<40 |
			uint64(generation)<<24 |
			uint64(slot),
	), nil
}

func DecodeHandle(handle Handle) (HandleParts, error) {
	if handle == 0 {
		return HandleParts{}, &Error{Kind: ErrorInvalidHandle}
	}
	parts := HandleParts{
		Owner:      uint32(uint64(handle) >> 40),
		Generation: uint16(uint64(handle) >> 24),
		Slot:       uint32(uint64(handle) & handlePartMask),
	}
	if parts.Owner == 0 || parts.Generation == 0 || parts.Slot == 0 {
		return HandleParts{}, &Error{
			Kind:   ErrorInvalidHandle,
			Handle: handle,
			Detail: fmt.Sprintf("malformed bits %#x", uint64(handle)),
		}
	}
	return parts, nil
}
