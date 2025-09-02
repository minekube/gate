package floodgate

import (
	"fmt"
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
	Version      string // Floodgate version
	Username     string // Bedrock username
	Xuid         int64  // Xbox User ID
	DeviceOS     int    // Device operating system
	Language     string // Client language
	UIProfile    int    // UI profile (classic/pocket)
	InputMode    int    // Input method (touch/keyboard/controller)
	IP           string // Player IP address
	LinkedPlayer string // Linked Java account (if any)
	Proxy        bool   // Whether player is behind a proxy
	SubscribeID  string // Subscribe ID for linking
	VerifyCode   string // Verification code for linking
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

// ReadHostname extracts Bedrock player data from a hostname.
// The hostname format is: original_hostname\x00encrypted_data[:port]
func (f *Floodgate) ReadHostname(hostname string) (string, *BedrockData, error) {
	parts := strings.Split(hostname, "\u0000")
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("invalid hostname format: %s", hostname)
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
		DeviceOS:     deviceOS,
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

// JavaUuid generates a Java Edition UUID from the Bedrock XUID.
// This creates a deterministic UUID that's consistent across sessions.
func (d *BedrockData) JavaUuid() (uuid.UUID, error) {
	xuid16 := strconv.FormatInt(d.Xuid, 16)

	// Pad with zeros if needed to ensure proper format
	for len(xuid16) < 12 {
		xuid16 = "0" + xuid16
	}

	// Create UUID in format: 00000000-0000-0000-000X-XXXXXXXXXXXX
	uuidStr := fmt.Sprintf("00000000-0000-0000-000%s-%s", xuid16[0:1], xuid16[1:])
	return uuid.Parse(uuidStr)
}
