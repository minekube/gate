package floodgate

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// Floodgate protocol constants
const (
	IV_LENGTH      = 12
	TAG_BIT_LENGTH = 128
	MAGIC          = 0x3E
	SPLITTER       = 0x21
	VERSION        = 0
	IDENTIFIER     = "^Floodgate^"
)

var (
	IDENTIFIER_BYTES = []byte(IDENTIFIER)
	HEADER           = IDENTIFIER + string(rune(VERSION+MAGIC))
)

// AesCipher provides AES encryption/decryption for Floodgate data.
// This implements the exact Floodgate cipher specification.
type AesCipher struct {
	key []byte
}

// NewAesCipher creates a new AES cipher with the given key.
// The key should be the raw bytes from the Floodgate key.pem file.
func NewAesCipher(key []byte) (*AesCipher, error) {
	// Floodgate supports AES-128, AES-192, and AES-256
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, errors.New("invalid key length for AES: must be 16, 24, or 32 bytes")
	}

	return &AesCipher{key: key}, nil
}

// Decrypt decrypts Floodgate-formatted ciphertext.
// The format is: ^Floodgate^VERSION+MAGIC + base64(IV) + SPLITTER + base64(ciphertext)
func (c *AesCipher) Decrypt(cipherTextWithIv []byte) ([]byte, error) {
	if len(cipherTextWithIv) < len(HEADER)+IV_LENGTH+1 {
		return nil, errors.New("invalid ciphertext length")
	}

	// Verify Floodgate header
	headerLen := len(HEADER)
	if !bytes.HasPrefix(cipherTextWithIv, []byte(HEADER)) {
		return nil, errors.New("invalid Floodgate header")
	}

	// Extract data after header
	data := cipherTextWithIv[headerLen:]

	// Find the splitter that separates IV from ciphertext
	splitIndex := bytes.IndexByte(data, SPLITTER)
	if splitIndex == -1 {
		return nil, errors.New("invalid format: missing splitter")
	}

	// Decode base64-encoded IV and ciphertext
	ivB64 := data[:splitIndex]
	cipherTextB64 := data[splitIndex+1:]

	iv, err := base64.StdEncoding.DecodeString(string(ivB64))
	if err != nil {
		return nil, fmt.Errorf("failed to decode IV: %w", err)
	}

	cipherText, err := base64.StdEncoding.DecodeString(string(cipherTextB64))
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the data
	plainText, err := gcm.Open(nil, iv, cipherText, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plainText, nil
}

// Encrypt encrypts data in Floodgate format (for completeness, though not used in proxy).
func (c *AesCipher) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random IV
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt the data
	ciphertext := gcm.Seal(nil, iv, plaintext, nil)

	// Format as Floodgate expects: header + base64(IV) + splitter + base64(ciphertext)
	ivB64 := base64.StdEncoding.EncodeToString(iv)
	ciphertextB64 := base64.StdEncoding.EncodeToString(ciphertext)

	result := []byte(HEADER)
	result = append(result, []byte(ivB64)...)
	result = append(result, SPLITTER)
	result = append(result, []byte(ciphertextB64)...)

	return result, nil
}
