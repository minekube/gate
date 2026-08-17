package floodgate

import (
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.minekube.com/gate/pkg/util/uuid"
)

// Floodgate handles Bedrock player authentication and data extraction.
type Floodgate struct {
	cipher *AesCipher
}

// BedrockData contains comprehensive information about a Bedrock player.
// This matches the Floodgate protocol specification.
type BedrockData struct {
	Version      string   // Floodgate version
	Username     string   // Bedrock username
	Xuid         int64    // Xbox User ID
	DeviceOS     DeviceOS // Device operating system
	Language     string   // Client language
	UIProfile    int      // UI profile (classic/pocket)
	InputMode    int      // Input method (touch/keyboard/controller)
	IP           string   // Player IP address
	LinkedPlayer string   // Linked Java account (if any)
	Proxy        bool     // Whether player is behind a proxy
	SubscribeID  string   // Subscribe ID for linking
	VerifyCode   string   // Verification code for linking
}

// NewFloodgate creates a new Floodgate instance with the given encryption key.
func NewFloodgate(key []byte) (*Floodgate, error) {
	cipher, err := NewAesCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	return &Floodgate{cipher: cipher}, nil
}

// Decrypt decrypts the given data using the Floodgate cipher.
func (f *Floodgate) Decrypt(data []byte) ([]byte, error) {
	return f.cipher.Decrypt(data)
}

// Encrypt encrypts the given data using the Floodgate cipher.
func (f *Floodgate) Encrypt(data []byte) ([]byte, error) {
	return f.cipher.Encrypt(data)
}

// ReadHostname extracts Bedrock player data from a hostname.
// The hostname format is: original_hostname\x00encrypted_data[:port]
func (f *Floodgate) ReadHostname(hostname string) (string, *BedrockData, error) {
	parts := strings.Split(hostname, "\u0000")
	if len(parts) != 2 {
		// The raw hostname can embed Floodgate identity data and must never
		// appear in the error; report only structural information.
		return "", nil, fmt.Errorf("invalid hostname format: expected 2 NUL-separated parts, got %d (hostname length %d)",
			len(parts), len(hostname))
	}

	originalHostname := parts[0]
	data := parts[1]

	// Remove port if present
	if strings.Contains(data, ":") {
		data = strings.Split(data, ":")[0]
	}

	// Decrypt the Bedrock data
	bedrockDataBytes, err := f.Decrypt([]byte(data))
	if err != nil {
		return "", nil, fmt.Errorf("failed to decrypt bedrock data: %w", err)
	}

	// Parse the decrypted data
	bedrockData, err := ReadBedrockData(string(bedrockDataBytes))
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse bedrock data: %w", err)
	}

	return originalHostname, bedrockData, nil
}

// WriteHostname encodes Bedrock player data into a Floodgate hostname payload.
func (f *Floodgate) WriteHostname(originalHostname string, d *BedrockData) (string, error) {
	if d == nil {
		return "", fmt.Errorf("bedrock data must not be nil")
	}
	if strings.ContainsRune(originalHostname, '\x00') {
		return "", fmt.Errorf("original hostname must not contain NUL")
	}
	fields := []string{
		d.Version,
		d.Username,
		strconv.FormatInt(d.Xuid, 10),
		strconv.Itoa(d.DeviceOS.ID),
		d.Language,
		strconv.Itoa(d.UIProfile),
		strconv.Itoa(d.InputMode),
		d.IP,
		d.LinkedPlayer,
		boolString(d.Proxy),
		d.SubscribeID,
		d.VerifyCode,
	}
	for _, field := range fields {
		if strings.ContainsRune(field, '\x00') {
			return "", fmt.Errorf("bedrock data fields must not contain NUL")
		}
	}
	data := strings.Join(fields, "\x00")

	encrypted, err := f.Encrypt([]byte(data))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\x00%s", originalHostname, encrypted), nil
}

func boolString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// ReadBedrockData parses the decrypted Bedrock data string.
// The format follows Floodgate's protocol: 12 null-separated fields.
func ReadBedrockData(data string) (*BedrockData, error) {
	parts := strings.Split(data, "\u0000")
	if len(parts) != 12 {
		return nil, fmt.Errorf("invalid bedrock data format: expected 12 parts, got %d", len(parts))
	}

	username := parts[1]
	if username == "" {
		return nil, fmt.Errorf("invalid username")
	}

	xuid, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid xuid: %w", err)
	}
	if xuid == 0 {
		return nil, fmt.Errorf("invalid xuid: cannot be 0")
	}

	deviceOS, err := strconv.Atoi(parts[3])
	if err != nil {
		return nil, fmt.Errorf("invalid device OS: %w", err)
	}

	uiProfile, err := strconv.Atoi(parts[5])
	if err != nil {
		return nil, fmt.Errorf("invalid UI profile: %w", err)
	}

	inputMode, err := strconv.Atoi(parts[6])
	if err != nil {
		return nil, fmt.Errorf("invalid input mode: %w", err)
	}

	return &BedrockData{
		Version:      parts[0],
		Username:     username,
		Xuid:         xuid,
		DeviceOS:     DeviceOSFromID(deviceOS),
		Language:     parts[4],
		UIProfile:    uiProfile,
		InputMode:    inputMode,
		IP:           parts[7],
		LinkedPlayer: parts[8],
		Proxy:        parts[9] == "1",
		SubscribeID:  parts[10],
		VerifyCode:   parts[11],
	}, nil
}

// LinkedPlayer is a parsed Floodgate linked Java account from the handshake
// triplet. It serializes on the wire as "javaUsername;javaUUID;bedrockUUID".
// An absent link is serialized by Floodgate as the literal string "null".
type LinkedPlayer struct {
	JavaUsername string
	JavaUUID     uuid.UUID
	BedrockUUID  uuid.UUID
}

// ParseLinkedPlayer parses the Floodgate LinkedPlayer field (field index 8 of
// the BedrockData wire format) into a LinkedPlayer. It returns nil for an
// absent link ("" or the literal "null") and for any malformed triplet,
// matching Floodgate's LinkedPlayer.fromString semantics (exactly three
// ';'-separated parts, both UUIDs parseable, non-empty Java username).
func ParseLinkedPlayer(raw string) *LinkedPlayer {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	if len(parts) != 3 {
		return nil
	}
	javaUsername := parts[0]
	if javaUsername == "" {
		return nil
	}
	javaUUID, err := uuid.Parse(parts[1])
	if err != nil {
		return nil
	}
	bedrockUUID, err := uuid.Parse(parts[2])
	if err != nil {
		return nil
	}
	return &LinkedPlayer{
		JavaUsername: javaUsername,
		JavaUUID:     javaUUID,
		BedrockUUID:  bedrockUUID,
	}
}

// FloodgateJavaUuid returns the UUID Floodgate derives from an XUID
// (Utils.getJavaUuid: new UUID(0, xuid)). It is the Bedrock side UUID stored
// in a LinkedPlayer triplet and is how Floodgate identifies a Bedrock
// connection, distinct from JavaUuid which is Gate's own deterministic
// XUID-derived profile UUID.
func (d *BedrockData) FloodgateJavaUuid() uuid.UUID {
	var u uuid.UUID
	// new UUID(0, xuid): most-significant 64 bits zero, XUID as the
	// least-significant 64 bits (big-endian).
	u[8] = byte(d.Xuid >> 56)
	u[9] = byte(d.Xuid >> 48)
	u[10] = byte(d.Xuid >> 40)
	u[11] = byte(d.Xuid >> 32)
	u[12] = byte(d.Xuid >> 24)
	u[13] = byte(d.Xuid >> 16)
	u[14] = byte(d.Xuid >> 8)
	u[15] = byte(d.Xuid)
	return u
}

// JavaUuid generates a Java Edition UUID from the Bedrock XUID.
// This creates a deterministic UUID that's consistent across sessions.
func (d *BedrockData) JavaUuid() (uuid.UUID, error) {
	// Namespaced deterministic UUID (v5-like) based on XUID
	h := sha1.New()
	h.Write([]byte("FloodgateXUID:"))
	h.Write([]byte(strconv.FormatInt(d.Xuid, 10)))
	sum := h.Sum(nil)
	if len(sum) < 16 {
		return uuid.Nil, fmt.Errorf("invalid hash length")
	}
	// Set version (5) and variant per RFC 4122
	sum[6] = (sum[6] & 0x0f) | (5 << 4)
	sum[8] = (sum[8] & 0x3f) | 0x80
	return uuid.FromBytes(sum[:16])
}

// GenerateKey generates a new 16-byte AES-128 key compatible with Floodgate.
// This matches Floodgate's AesKeyProducer.KEY_SIZE = 128 bits.
func GenerateKey() ([]byte, error) {
	key := make([]byte, 16) // 128 bits
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// GenerateKeyToFile generates a new Floodgate key and writes it to the specified path.
// Creates parent directories if needed and sets secure file permissions (0600).
func GenerateKeyToFile(keyPath string) error {
	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Generate the key
	key, err := GenerateKey()
	if err != nil {
		return err
	}

	// Write key to file with secure permissions
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	return nil
}
