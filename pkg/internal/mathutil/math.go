package mathutil

import (
	"bytes"
	"math"
)

// FloorDiv returns the floor of the quotient of a/b.
func FloorDiv(a, b int) int {
	return floorDiv(a, b)
	//return int(math.Floor(float64(a) / float64(b)))
}

func floorDiv(x, y int) int {
	if x == math.MinInt && y == -1 {
		return math.MinInt // handle integer overflow case
	}
	if x > 0 && y > 0 || x < 0 && y < 0 {
		// signs of x and y are the same
		return x / y // integer division in Go rounds toward 0
	} else {
		// signs of x and y are different
		return x/y - 1 // round toward negative infinity
	}
}

// BitSet represents a set of Bytes that can be set and cleared.
type BitSet struct {
	// The underlying byte slice that holds the bits.
	Bytes []byte
}

// NewBitSet creates a new BitSet with the given length.
func NewBitSet(length int) *BitSet {
	return &BitSet{Bytes: make([]byte, (length+7)/8)}
}

// SetBool sets the bit at the given index to the given value. If the index is
// greater than the current length of the BitSet, the BitSet is automatically
// grown to accommodate the new index.
func (b *BitSet) SetBool(index int, value bool) {
	if value {
		b.Set(index)
	} else {
		b.Clear(index)
	}
}

// Set sets the bit at the given index to 1. If the index is greater than the
// current length of the BitSet, the BitSet is automatically grown to accommodate
// the new index.
func (b *BitSet) Set(index int) {
	if index >= len(b.Bytes)*8 {
		// Grow the underlying slice to accommodate the new index.
		newBits := make([]byte, index/8+1)
		copy(newBits, b.Bytes)
		b.Bytes = newBits
	}
	b.Bytes[index/8] |= 1 << (index % 8)
}

// Clear sets the bit at the given index to 0. If the index is greater than the
// current length of the BitSet, the BitSet is automatically grown to accommodate
// the new index.
func (b *BitSet) Clear(index int) {
	if index >= len(b.Bytes)*8 {
		// Grow the underlying slice to accommodate the new index.
		newBits := make([]byte, index/8+1)
		copy(newBits, b.Bytes)
		b.Bytes = newBits
	}
	b.Bytes[index/8] &= ^(1 << (index % 8))
}

// Test returns the value of the bit at the given index. If the index is greater
// than the current length of the BitSet, false is returned.
func (b *BitSet) Test(index int) bool {
	if index >= len(b.Bytes)*8 {
		return false
	}
	return b.Bytes[index/8]&(1<<(index%8)) != 0
}

// String returns a string representation of the BitSet.
func (b *BitSet) String() string {
	var buffer bytes.Buffer
	for i := 0; i < len(b.Bytes)*8; i++ {
		if b.Test(i) {
			buffer.WriteRune('1')
		} else {
			buffer.WriteRune('0')
		}
	}
	return buffer.String()
}

// Empty returns true if this BitSet contains no Bytes that are set to true.
func (b *BitSet) Empty() bool {
	for i := 0; i < len(b.Bytes); i++ {
		if b.Bytes[i] != 0 {
			return false
		}
	}
	return true
}
