package util

import (
	"bytes"
	"fmt"
	"math"
	"testing"
)

// TestVarIntNegativeValues tests that VarInt properly handles negative values
// This is critical for Forge/NeoForge compatibility, particularly with CrossStitch mod
func TestVarIntNegativeValues(t *testing.T) {
	testCases := []int{
		-256,        // CrossStitch mod_argument identifier
		-1,          // Simple negative
		0,           // Zero
		127,         // Positive within single byte
		128,         // Positive requiring multiple bytes
		2147483647,  // Max positive int32
		-2147483648, // Min negative int32
	}

	for _, testValue := range testCases {
		t.Run(fmt.Sprintf("VarInt_%d", testValue), func(t *testing.T) {
			var buf bytes.Buffer

			// Write the value
			err := WriteVarInt(&buf, testValue)
			if err != nil {
				t.Fatalf("Failed to write VarInt %d: %v", testValue, err)
			}

			// Read it back
			readValue, err := ReadVarInt(&buf)
			if err != nil {
				t.Fatalf("Failed to read VarInt %d: %v", testValue, err)
			}

			// Compare
			if readValue != testValue {
				t.Errorf("VarInt mismatch: wrote %d, read %d", testValue, readValue)
			}
		})
	}
}

// TestCrossStitchCompatibility specifically tests the CrossStitch mod case
func TestCrossStitchCompatibility(t *testing.T) {
	// This is the specific case that causes "identifier not found" errors
	crossStitchId := -256

	var buf bytes.Buffer
	err := WriteVarInt(&buf, crossStitchId)
	if err != nil {
		t.Fatalf("Failed to write CrossStitch mod argument ID: %v", err)
	}

	readValue, err := ReadVarInt(&buf)
	if err != nil {
		t.Fatalf("Failed to read CrossStitch mod argument ID: %v", err)
	}

	if readValue != crossStitchId {
		t.Errorf("CrossStitch mod argument ID mismatch: wrote %d, read %d", crossStitchId, readValue)
	}

	// Verify the bytes match expected encoding
	buf.Reset()
	err = WriteVarInt(&buf, crossStitchId)
	if err != nil {
		t.Fatalf("Failed to write CrossStitch mod argument ID for byte verification: %v", err)
	}
	actualBytes := buf.Bytes()
	expected := []byte{128, 254, 255, 255, 15} // Expected encoding for -256

	if !bytes.Equal(actualBytes, expected) {
		t.Errorf("CrossStitch mod argument ID encoding mismatch: got %v, expected %v", actualBytes, expected)
	}
}

// varIntBytes calculates the number of bytes needed to encode a VarInt
// Ported from Velocity's ProtocolUtils.varIntBytes()
func varIntBytes(value int) int {
	// Convert to uint32 for bit operations, matching Java's behavior
	uvalue := uint32(value)

	// Count leading zeros and use lookup table approach like Velocity
	leadingZeros := 0
	if uvalue == 0 {
		return 1 // Special case for 0
	}

	// Count leading zeros manually since Go doesn't have Integer.numberOfLeadingZeros
	temp := uvalue
	for temp != 0 {
		leadingZeros++
		temp >>= 1
	}
	leadingZeros = 32 - leadingZeros

	// Use Velocity's formula: ceil((31 - (leadingZeros - 1)) / 7)
	if leadingZeros == 32 {
		return 1 // Special case for 0
	}
	return int(math.Ceil(float64(31-(leadingZeros-1)) / 7.0))
}

// writeVarIntOld implements the traditional VarInt encoding for testing
// Ported from Velocity's writeVarIntOld method
func writeVarIntOld(buf *bytes.Buffer, value int) {
	uvalue := uint32(value)
	for {
		if (uvalue & 0xFFFFFF80) == 0 {
			buf.WriteByte(byte(uvalue))
			return
		}
		buf.WriteByte(byte(uvalue&0x7F | 0x80))
		uvalue >>= 7
	}
}

// readVarIntOld implements the traditional VarInt decoding for testing
// Ported from Velocity's oldReadVarIntSafely method
func readVarIntOld(buf *bytes.Buffer) int {
	i := 0
	maxRead := min(5, buf.Len())
	for j := 0; j < maxRead; j++ {
		if buf.Len() == 0 {
			return math.MinInt32
		}
		k := int(buf.Next(1)[0])
		i |= (k & 0x7F) << (j * 7)
		if (k & 0x80) != 128 {
			return i
		}
	}
	return math.MinInt32
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestNegativeVarIntBytes tests byte length calculation for negative values
// Ported from Velocity's negativeVarIntBytes test
func TestNegativeVarIntBytes(t *testing.T) {
	if varIntBytes(-1) != 5 {
		t.Errorf("Expected -1 to require 5 bytes, got %d", varIntBytes(-1))
	}
	if varIntBytes(math.MinInt32) != 5 {
		t.Errorf("Expected MinInt32 to require 5 bytes, got %d", varIntBytes(math.MinInt32))
	}
}

// TestZeroVarIntBytes tests byte length calculation for zero and small positive values
// Ported from Velocity's zeroVarIntBytes test
func TestZeroVarIntBytes(t *testing.T) {
	if varIntBytes(0) != 1 {
		t.Errorf("Expected 0 to require 1 byte, got %d", varIntBytes(0))
	}
	if varIntBytes(1) != 1 {
		t.Errorf("Expected 1 to require 1 byte, got %d", varIntBytes(1))
	}
}

// TestConsistencyAcrossNumberBits tests VarInt encoding consistency across bit boundaries
// Ported from Velocity's ensureConsistencyAcrossNumberBits test
func TestConsistencyAcrossNumberBits(t *testing.T) {
	for i := 0; i <= 31; i++ {
		number := (1 << i) - 1
		expected := conventionalWrittenBytes(number)
		actual := varIntBytes(number)
		if actual != expected {
			t.Errorf("Mismatch with %d-bit number %d: expected %d bytes, got %d", i, number, expected, actual)
		}
	}
}

// conventionalWrittenBytes calculates bytes using traditional method
// Ported from Velocity's conventionalWrittenBytes method
func conventionalWrittenBytes(value int) int {
	wouldBeWritten := 0
	uvalue := uint32(value)
	for {
		if (uvalue & ^uint32(0x7F)) == 0 {
			wouldBeWritten++
			return wouldBeWritten
		}
		wouldBeWritten++
		uvalue >>= 7
	}
}

// TestPositiveVarIntRoundtrip tests positive VarInt values with incremental steps
// Ported from Velocity's testPositiveOld test
func TestPositiveVarIntRoundtrip(t *testing.T) {
	for i := 0; i >= 0 && i < 1000000; i += 127 {
		var buf bytes.Buffer
		err := WriteVarInt(&buf, i)
		if err != nil {
			t.Fatalf("Failed to write VarInt %d: %v", i, err)
		}

		readValue, err := ReadVarInt(&buf)
		if err != nil {
			t.Fatalf("Failed to read VarInt %d: %v", i, err)
		}

		if readValue != i {
			t.Errorf("VarInt roundtrip failed for %d: got %d", i, readValue)
		}

		// Prevent infinite loop
		if i > math.MaxInt32-127 {
			break
		}
	}
}

// TestNegativeVarIntRoundtrip tests negative VarInt values with incremental steps
// Ported from Velocity's testNegativeOld test
func TestNegativeVarIntRoundtrip(t *testing.T) {
	for i := 0; i <= 0 && i > -1000000; i -= 127 {
		var buf bytes.Buffer
		err := WriteVarInt(&buf, i)
		if err != nil {
			t.Fatalf("Failed to write VarInt %d: %v", i, err)
		}

		readValue, err := ReadVarInt(&buf)
		if err != nil {
			t.Fatalf("Failed to read VarInt %d: %v", i, err)
		}

		if readValue != i {
			t.Errorf("VarInt roundtrip failed for %d: got %d", i, readValue)
		}

		// Prevent infinite loop
		if i < math.MinInt32+127 {
			break
		}
	}
}

// TestBytesWrittenAtBitBoundaries tests that our implementation matches traditional encoding
// Ported from Velocity's testBytesWrittenAtBitBoundaries test
func TestBytesWrittenAtBitBoundaries(t *testing.T) {
	for bit := 0; bit <= 31; bit++ {
		number := (1 << bit) - 1

		// Test with our implementation
		var bufNew bytes.Buffer
		err := WriteVarInt(&bufNew, number)
		if err != nil {
			t.Fatalf("Failed to write VarInt %d: %v", number, err)
		}

		// Test with old implementation
		var bufOld bytes.Buffer
		writeVarIntOld(&bufOld, number)

		// Compare byte arrays
		newBytes := bufNew.Bytes()
		oldBytes := bufOld.Bytes()

		if !bytes.Equal(newBytes, oldBytes) {
			t.Errorf("Encoding of %d was invalid: new=%v, old=%v", number, newBytes, oldBytes)
		}

		// Test reading with both implementations
		bufNewCopy := bytes.NewBuffer(newBytes)
		readNew, err := ReadVarInt(bufNewCopy)
		if err != nil {
			t.Fatalf("Failed to read VarInt %d with new implementation: %v", number, err)
		}

		bufOldCopy := bytes.NewBuffer(oldBytes)
		readOld := readVarIntOld(bufOldCopy)

		if readNew != number {
			t.Errorf("New implementation read mismatch for %d: got %d", number, readNew)
		}
		if readOld != number && readOld != math.MinInt32 {
			t.Errorf("Old implementation read mismatch for %d: got %d", number, readOld)
		}
	}
}
