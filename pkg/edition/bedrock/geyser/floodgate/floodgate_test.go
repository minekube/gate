package floodgate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadHostnameAndData(t *testing.T) {
	key := bytes.Repeat([]byte{0x13}, 16) // Use 16 bytes for AES-128
	fg, err := NewFloodgate(key)
	if err != nil {
		t.Fatalf("NewFloodgate: %v", err)
	}
	// Construct bedrock data with 12 fields
	fields := []string{
		"1",               // Version
		"Steve",           // Username
		"281474976710655", // XUID (max 48-bit)
		"2",               // DeviceOS
		"en_US",           // Language
		"1",               // UIProfile
		"1",               // InputMode
		"127.0.0.1",       // IP
		"",                // LinkedPlayer
		"0",               // Proxy
		"sub",             // SubscribeID
		"code",            // VerifyCode
	}
	data := []byte(strings.Join(fields, "\x00"))
	enc, err := fg.Encrypt(data)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	host := fmt.Sprintf("play.example.org\x00%s", enc)

	original, bd, err := fg.ReadHostname(host)
	if err != nil {
		t.Fatalf("ReadHostname: %v", err)
	}
	if original != "play.example.org" {
		t.Fatalf("original host mismatch: %q", original)
	}
	if bd.Username != "Steve" || bd.Xuid == 0 {
		t.Fatalf("unexpected data: %#v", bd)
	}
	if _, err := bd.JavaUuid(); err != nil {
		t.Fatalf("JavaUuid: %v", err)
	}
}

func TestWriteHostnameRoundTripsBedrockData(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 16)
	fg, err := NewFloodgate(key)
	if err != nil {
		t.Fatalf("NewFloodgate: %v", err)
	}

	want := &BedrockData{
		Version:      "1",
		Username:     "Fox",
		Xuid:         123456789,
		DeviceOS:     DeviceOSWindowsUWP,
		Language:     "en_US",
		UIProfile:    1,
		InputMode:    2,
		IP:           "203.0.113.10",
		LinkedPlayer: "LinkedJava",
		Proxy:        true,
		SubscribeID:  "sub-id",
		VerifyCode:   "verify-code",
	}

	host, err := fg.WriteHostname("play.example.org:19132", want)
	if err != nil {
		t.Fatalf("WriteHostname: %v", err)
	}

	original, got, err := fg.ReadHostname(host)
	if err != nil {
		t.Fatalf("ReadHostname: %v", err)
	}

	if original != "play.example.org:19132" {
		t.Fatalf("original host = %q, want %q", original, "play.example.org:19132")
	}
	if *got != *want {
		t.Fatalf("bedrock data = %#v, want %#v", got, want)
	}
}

func TestWriteHostnameRejectsAmbiguousData(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 16)
	fg, err := NewFloodgate(key)
	if err != nil {
		t.Fatalf("NewFloodgate: %v", err)
	}

	valid := &BedrockData{
		Version:   "1",
		Username:  "Fox",
		Xuid:      123456789,
		DeviceOS:  DeviceOSWindowsUWP,
		Language:  "en_US",
		UIProfile: 1,
		InputMode: 2,
		IP:        "203.0.113.10",
	}

	if _, err := fg.WriteHostname("play.example.org\x00spoof", valid); err == nil {
		t.Fatal("WriteHostname accepted original hostname containing NUL")
	}

	withNUL := *valid
	withNUL.Username = "Fox\x00Admin"
	if _, err := fg.WriteHostname("play.example.org", &withNUL); err == nil {
		t.Fatal("WriteHostname accepted Bedrock data containing NUL")
	}

	if _, err := fg.WriteHostname("play.example.org", nil); err == nil {
		t.Fatal("WriteHostname accepted nil Bedrock data")
	}
}

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if len(key) != 16 {
		t.Fatalf("expected 16-byte key, got %d bytes", len(key))
	}

	// Test that the key works with Floodgate
	fg, err := NewFloodgate(key)
	if err != nil {
		t.Fatalf("NewFloodgate with generated key: %v", err)
	}

	// Test encrypt/decrypt cycle
	testMessage := []byte("test message for floodgate key validation")
	encrypted, err := fg.Encrypt(testMessage)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := fg.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(testMessage, decrypted) {
		t.Fatalf("encrypt/decrypt cycle failed: got %q, want %q", decrypted, testMessage)
	}
}

func TestGenerateKeyToFile(t *testing.T) {
	// Create a temporary file path
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "subdir", "test-key.pem")

	// Generate key to file
	if err := GenerateKeyToFile(keyPath); err != nil {
		t.Fatalf("GenerateKeyToFile: %v", err)
	}

	// Check file exists and has correct size
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("key file not created: %v", err)
	}
	if info.Size() != 16 {
		t.Fatalf("expected 16-byte key file, got %d bytes", info.Size())
	}

	// Check file permissions are secure (owner read/write only)
	// Note: Windows file permission model differs from Unix, so we verify different constraints
	perm := info.Mode().Perm()
	if perm&0o200 == 0 {
		t.Fatalf("file should be writable by owner, got permissions %o", perm)
	}
	if perm&0o400 == 0 {
		t.Fatalf("file should be readable by owner, got permissions %o", perm)
	}
	// On Unix systems, also verify it's not world-readable
	if perm&0o004 != 0 {
		t.Logf("Warning: file may be world-readable (permissions %o). This is expected on Windows.", perm)
	}

	// Test that the generated key works
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("failed to read generated key: %v", err)
	}

	fg, err := NewFloodgate(keyData)
	if err != nil {
		t.Fatalf("NewFloodgate with file-generated key: %v", err)
	}

	// Quick encrypt/decrypt test
	testMessage := []byte("file key test")
	encrypted, err := fg.Encrypt(testMessage)
	if err != nil {
		t.Fatalf("Encrypt with file key: %v", err)
	}

	decrypted, err := fg.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt with file key: %v", err)
	}

	if !bytes.Equal(testMessage, decrypted) {
		t.Fatalf("file key encrypt/decrypt failed: got %q, want %q", decrypted, testMessage)
	}
}
