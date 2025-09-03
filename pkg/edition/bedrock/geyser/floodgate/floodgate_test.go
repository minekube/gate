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
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
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
