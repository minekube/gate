package mathutil

import (
	"math"
	"strings"
)

// FloorDiv returns the floor of the quotient of a/b.
func FloorDiv(a, b int) int {
	return int(math.Floor(float64(a) / float64(b)))
}

// BitSet is a bit set.
type BitSet struct {
	Bytes []byte
}

// NewBitSet returns a new bit set with the given length.
func NewBitSet(length int) *BitSet {
	return &BitSet{Bytes: make([]byte, -FloorDiv(-length, 8))}
}

// Set sets the bit at the given index to the given value 0 or 1.
// It grows the bit set if necessary.
func (b *BitSet) Set(index int, value bool) BitSet {
	b.Grow(index + 1)
	pos := index / 8
	j := uint(index % 8)
	if value {
		b.Bytes[pos] |= byte(1) << j
	} else {
		b.Bytes[pos] &^= byte(1) << j
	}
	return *b
}

// Get returns the bit at the given index.
func (b *BitSet) Get(index int) bool {
	pos := index / 8
	j := uint(index % 8)
	return b.Bytes[pos]&(byte(1)<<j) != 0
}

// Grow grows the bit set to the given length.
func (b *BitSet) Grow(length int) {
	if length > len(b.Bytes)*8 {
		newBytes := make([]byte, -FloorDiv(-length, 8))
		copy(newBytes, b.Bytes)
		b.Bytes = newBytes
	}
}

// String returns a string representation of the bit set.
func (b BitSet) String() string {
	var sb strings.Builder
	for i := 0; i < len(b.Bytes)*8; i++ {
		if b.Get(i) {
			sb.WriteByte('1')
		} else {
			sb.WriteByte('0')
		}
	}
	return sb.String()
}

// Empty returns true if this BitSet contains no bits that are set to true.
func (b BitSet) Empty() bool {
	for _, v := range b.Bytes {
		if v != 0 {
			return false
		}
	}
	return true
}
