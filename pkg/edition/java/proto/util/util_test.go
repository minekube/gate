package util

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/util/uuid"
)

func TestVarInt(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		data    []byte
		wantVal int
		wantErr error
	}{
		{name: "single byte", data: []byte{0x01}, wantVal: 1},
		{name: "two bytes", data: []byte{0xAC, 0x02}, wantVal: 300},
		{name: "zero", data: []byte{0x00}, wantVal: 0},
		{
			name:    "max varint",
			data:    []byte{0xff, 0xff, 0xff, 0xff, 0x07},
			wantVal: 2147483647, // MaxInt32
		},
		{
			name:    "varint too big",
			data:    []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0x01},
			wantErr: errors.New("decode: VarInt is too big"),
		},
		{
			name:    "empty buffer",
			data:    []byte{},
			wantErr: io.EOF,
		},
		{
			name:    "incomplete varint",
			data:    []byte{0xff}, // Missing subsequent bytes
			wantErr: io.EOF,
		},
		{
			name:    "valid 5 byte varint",
			data:    []byte{0x80, 0x80, 0x80, 0x80, 0x01},
			wantVal: 268435456,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf := bytes.NewBuffer(tc.data)
			gotVal, gotErr := ReadVarInt(buf)

			if tc.wantErr != nil {
				require.Error(t, gotErr)
				// Check if the error message or type matches, depending on what's more appropriate
				// For "VarInt is too big", we check the exact error message.
				// For io.EOF, we check if it's an EOF error.
				if tc.name == "varint too big" || tc.name == "empty buffer" || tc.name == "incomplete varint" {
					require.EqualError(t, gotErr, tc.wantErr.Error())
				} else {
					require.ErrorIs(t, gotErr, tc.wantErr)
				}
			} else {
				require.NoError(t, gotErr)
				require.Equal(t, tc.wantVal, gotVal)
			}
		})
	}
}

func TestUTF(t *testing.T) {
	b := new(bytes.Buffer)
	require.NoError(t, WriteUTF(b, "test"))
	s, err := ReadUTF(b)
	require.NoError(t, err)
	require.Equal(t, "test", s)
}

func TestUUIDIntArray(t *testing.T) {
	t.Parallel()
	// Generate a random UUID
	id := uuid.New()

	// Create a buffer and write the UUID to it as an integer array
	buf := new(bytes.Buffer)
	err := WriteUUIDIntArray(buf, id)
	require.NoError(t, err)

	// Read the UUID from the buffer
	readID, err := ReadUUIDIntArray(buf)
	require.NoError(t, err)

	// The read UUID should be the same as the original UUID
	require.Equal(t, id, readID)
}

func FuzzReadVarInt(f *testing.F) {
	testCases := [][]byte{
		{0x01},
		{0xAC, 0x02},
		{0x00},
		{0xff, 0xff, 0xff, 0xff, 0x07},
		{0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, // too big
		{},                                   // empty
		{0xff},                               // incomplete
		{0x80, 0x80, 0x80, 0x80, 0x01},
	}
	for _, tc := range testCases {
		f.Add(tc)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		buf := bytes.NewBuffer(data)
		val, err := ReadVarInt(buf)

		// We expect an error if the VarInt is too big or incomplete
		// or if the input data is malformed in a way ReadVarInt can detect.
		// Otherwise, we expect the read to consume some bytes and potentially return a value.

		if err != nil {
			if !errors.Is(err, io.EOF) && err.Error() != "decode: VarInt is too big" {
				// If it's another type of error, we might want to investigate.
				// For now, we just ensure it doesn't panic.
				t.Fatalf("ReadVarInt returned an unexpected error: %v for input %x", err, data)
			}
			return // Error is expected in some cases
		}

		// If no error, ensure val is non-negative as VarInts typically represent sizes or counts
		if val < 0 {
			// This case should ideally be covered by the VarInt format itself,
			// as standard VarInts don't represent negative numbers directly
			// unless a zigzag encoding is used (which is not the case here).
			// The current ReadVarInt decodes into an int, so it *could* be negative
			// if a 5-byte sequence results in a value > MaxInt32 when interpreted as uint32.
			// For example: []byte{0xff, 0xff, 0xff, 0xff, 0x0f} would be -1 if read as int32.
			// Let's verify this behavior or decide if ReadVarInt should error.
			// The current implementation of ReadVarInt casts to int(result) where result is int32.
			// If the 5th byte makes the int32 value negative, it will be negative.
			t.Logf("ReadVarInt returned a negative value: %d for input %x. This may be expected due to int(int32) conversion for certain 5-byte sequences.", val, data)
		}

		// Check remaining bytes if any
		// If the read was successful (no error), some bytes should have been consumed
		// unless the input was empty and ReadVarInt errored out before this check.
		if len(data) > 0 && buf.Len() == len(data) && err == nil {
			t.Errorf("ReadVarInt consumed no bytes from non-empty buffer without error, val: %d", val)
		}
	})
}

func BenchmarkReadVarInt(b *testing.B) {
	data := []byte{0xAC, 0x02} // 300
	reader := bytes.NewReader(data)
	b.ResetTimer()
	for b.Loop() {
		_, _ = reader.Seek(0, io.SeekStart) // Reset reader for each iteration
		_, err := ReadVarInt(reader)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestReadVarIntReturnN(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedVal int
		expectedN   int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "single byte varint",
			input:       []byte{0x01},
			expectedVal: 1,
			expectedN:   1,
			expectError: false,
		},
		{
			name:        "multi-byte varint (150)",
			input:       []byte{0x96, 0x01},
			expectedVal: 150,
			expectedN:   2,
			expectError: false,
		},
		{
			name:        "max varint value (2147483647)",
			input:       []byte{0xff, 0xff, 0xff, 0xff, 0x07},
			expectedVal: 2147483647,
			expectedN:   5,
			expectError: false,
		},
		{
			name:        "varint too large",
			input:       []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x01}, // 6 bytes, too large
			expectedVal: 0,
			expectedN:   5, // Should read up to 5 bytes before determining it's too large
			expectError: true,
			errorMsg:    "decode: VarInt is too big",
		},
		{
			name:        "incomplete varint - EOF after 1 byte of 2-byte varint",
			input:       []byte{0x96}, // Represents 150, but is incomplete
			expectedVal: 0,
			expectedN:   1, // Read 1 byte before EOF
			expectError: true,
			errorMsg:    "EOF",
		},
		{
			name:        "incomplete varint - EOF after 2 bytes of 5-byte varint",
			input:       []byte{0xff, 0xff}, // Incomplete
			expectedVal: 0,
			expectedN:   2, // Read 2 bytes before EOF
			expectError: true,
			errorMsg:    "EOF",
		},
		{
			name:        "zero value",
			input:       []byte{0x00},
			expectedVal: 0,
			expectedN:   1,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewBuffer(tt.input)
			val, n, err := ReadVarIntReturnN(buf)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					require.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedVal, val)
			}
			require.Equal(t, tt.expectedN, n)
		})
	}
}
