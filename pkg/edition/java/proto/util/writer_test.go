package util

import (
	"bytes"
	"math"
	"testing"

	"go.minekube.com/common/minecraft/key"
)

func TestReadKeyRejectsInvalidResourceLocations(t *testing.T) {
	for _, raw := range []string{
		"MineKube:cookie",
		"minecraft:BadCookie",
		"mine kube:cookie",
		"..:cookie",
	} {
		t.Run(raw, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteString(&buf, raw); err != nil {
				t.Fatalf("failed to write raw key string: %v", err)
			}

			if got, err := ReadKey(&buf); err == nil {
				t.Fatalf("ReadKey(%q) = %q, want error", raw, got)
			}
		})
	}
}

func TestReadKeyDefaultsMissingNamespaceToMinecraft(t *testing.T) {
	for raw, want := range map[string]key.Key{
		"cookie":  key.New(key.MinecraftNamespace, "cookie"),
		":cookie": key.New(key.MinecraftNamespace, "cookie"),
	} {
		t.Run(raw, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteString(&buf, raw); err != nil {
				t.Fatalf("failed to write raw key string: %v", err)
			}

			got, err := ReadKey(&buf)
			if err != nil {
				t.Fatalf("ReadKey(%q) returned error: %v", raw, err)
			}
			if got.String() != want.String() {
				t.Fatalf("ReadKey(%q) = %q, want %q", raw, got, want)
			}
		})
	}
}

func TestWriteKeyRejectsInvalidResourceLocations(t *testing.T) {
	for _, invalid := range []key.Key{
		key.New("MineKube", "cookie"),
		key.New("minecraft", "BadCookie"),
		key.New("mine kube", "cookie"),
		key.New("..", "cookie"),
	} {
		t.Run(invalid.String(), func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteKey(&buf, invalid); err == nil {
				t.Fatalf("WriteKey(%q) succeeded, want error", invalid)
			}
		})
	}
}

// TestWriteBytes17_NonExtended tests WriteBytes17 with allowExtended=false
// This is the case that was failing for 1.7.x clients with encryption requests
func TestWriteBytes17_NonExtended(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectError bool
		description string
	}{
		{
			name:        "small_array",
			data:        make([]byte, 100),
			expectError: false,
			description: "Small array should work",
		},
		{
			name:        "max_int8_boundary",
			data:        make([]byte, 127), // math.MaxInt8
			expectError: false,
			description: "Array at old limit (127) should work",
		},
		{
			name:        "above_old_limit",
			data:        make([]byte, 162), // Size from issue #533
			expectError: false,
			description: "Array of 162 bytes (from issue #533) should work with new limit",
		},
		{
			name:        "large_valid_array",
			data:        make([]byte, 1000),
			expectError: false,
			description: "Larger array should work with new limit",
		},
		{
			name:        "max_int16_boundary",
			data:        make([]byte, math.MaxInt16), // 32767
			expectError: false,
			description: "Array at new limit (32767) should work",
		},
		{
			name:        "above_new_limit",
			data:        make([]byte, math.MaxInt16+1), // 32768
			expectError: true,
			description: "Array above new limit should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteBytes17(&buf, tt.data, false) // allowExtended = false

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, but got none", tt.description)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %s: %v", tt.description, err)
				}
			}
		})
	}
}

// TestWriteBytes17_Extended tests WriteBytes17 with allowExtended=true
func TestWriteBytes17_Extended(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectError bool
		description string
	}{
		{
			name:        "small_array_extended",
			data:        make([]byte, 100),
			expectError: false,
			description: "Small array should work in extended mode",
		},
		{
			name:        "large_array_extended",
			data:        make([]byte, 50000),
			expectError: false,
			description: "Large array should work in extended mode",
		},
		{
			name:        "forge_max_boundary",
			data:        make([]byte, ForgeMaxArrayLength),
			expectError: false,
			description: "Array at forge limit should work",
		},
		{
			name:        "above_forge_limit",
			data:        make([]byte, ForgeMaxArrayLength+1),
			expectError: true,
			description: "Array above forge limit should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteBytes17(&buf, tt.data, true) // allowExtended = true

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, but got none", tt.description)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %s: %v", tt.description, err)
				}
			}
		})
	}
}

// TestWriteBytes17_1_7_x_EncryptionRequest tests the specific case from issue #533
// where a 1.7.x client fails to login due to encryption request packet size
func TestWriteBytes17_1_7_x_EncryptionRequest(t *testing.T) {
	// Simulate a typical RSA public key size that would be used in encryption requests
	// RSA 1024-bit public key in DER format is typically around 162 bytes
	publicKey := make([]byte, 162) // Size from the actual error in issue #533
	verifyToken := make([]byte, 4) // Typical verify token size

	var buf bytes.Buffer

	// Test public key writing (this was failing before the fix)
	err := WriteBytes17(&buf, publicKey, false)
	if err != nil {
		t.Errorf("Failed to write public key for 1.7.x client: %v", err)
	}

	// Test verify token writing
	err = WriteBytes17(&buf, verifyToken, false)
	if err != nil {
		t.Errorf("Failed to write verify token for 1.7.x client: %v", err)
	}

	// Verify that data was actually written
	if buf.Len() == 0 {
		t.Error("No data was written to buffer")
	}
}
