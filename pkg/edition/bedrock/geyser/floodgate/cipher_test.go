package floodgate

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestAesCipher_RoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	c, err := NewAesCipher(key)
	if err != nil {
		t.Fatalf("NewAesCipher: %v", err)
	}
	plain := []byte("hello floodgate")
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// quick header structural check
	if !bytes.HasPrefix(enc, []byte(HEADER)) {
		t.Fatalf("missing header")
	}
	// ensure there is a splitter and two base64 parts
	s := enc[len(HEADER):]
	parts := bytes.SplitN(s, []byte{SPLITTER}, 2)
	if len(parts) != 2 {
		t.Fatalf("missing splitter")
	}
	if _, err := base64.StdEncoding.DecodeString(string(parts[0])); err != nil {
		t.Fatalf("iv b64: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(string(parts[1])); err != nil {
		t.Fatalf("ct b64: %v", err)
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(plain, dec) {
		t.Fatalf("mismatch: got %q want %q", dec, plain)
	}
}
