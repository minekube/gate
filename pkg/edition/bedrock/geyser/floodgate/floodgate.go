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

// DeviceOS represents the operating system of a device.
type DeviceOS struct {
	ID   int
	Name string
}

// String implements fmt.Stringer.
func (d DeviceOS) String() string {
	return d.Name
}

// DeviceOS constants
// See https://github.com/GeyserMC/Geyser/blob/master/common/src/main/java/org/geysermc/floodgate/util/DeviceOs.java#L35
var (
	DeviceOSUnknown      = DeviceOS{ID: 0, Name: "Unknown"}
	DeviceOSAndroid      = DeviceOS{ID: 1, Name: "Android"}
	DeviceOSIOS          = DeviceOS{ID: 2, Name: "iOS"}
	DeviceOSMacOS        = DeviceOS{ID: 3, Name: "macOS"}
	DeviceOSAmazon       = DeviceOS{ID: 4, Name: "Amazon"}
	DeviceOSGearVR       = DeviceOS{ID: 5, Name: "Gear VR"}
	DeviceOSHololens     = DeviceOS{ID: 6, Name: "Hololens"} // Deprecated
	DeviceOSWindowsUWP   = DeviceOS{ID: 7, Name: "Windows"}
	DeviceOSWindowsX86   = DeviceOS{ID: 8, Name: "Windows x86"}
	DeviceOSDedicated    = DeviceOS{ID: 9, Name: "Dedicated"}
	DeviceOSAppleTV      = DeviceOS{ID: 10, Name: "Apple TV"}    // Deprecated
	DeviceOSPlayStation  = DeviceOS{ID: 11, Name: "PlayStation"} // All PlayStation platforms
	DeviceOSSwitch       = DeviceOS{ID: 12, Name: "Switch"}
	DeviceOSXbox         = DeviceOS{ID: 13, Name: "Xbox"}
	DeviceOSWindowsPhone = DeviceOS{ID: 14, Name: "Windows Phone"} // Deprecated
	DeviceOSLinux        = DeviceOS{ID: 15, Name: "Linux"}
)

// DeviceOSes is a list of all DeviceOSes.
var DeviceOSes = []DeviceOS{
	DeviceOSUnknown,
	DeviceOSAndroid,
	DeviceOSIOS,
	DeviceOSMacOS,
	DeviceOSAmazon,
	DeviceOSGearVR,
	DeviceOSHololens,
	DeviceOSWindowsUWP,
	DeviceOSWindowsX86,
	DeviceOSDedicated,
	DeviceOSAppleTV,
	DeviceOSPlayStation,
	DeviceOSSwitch,
	DeviceOSXbox,
	DeviceOSWindowsPhone,
	DeviceOSLinux,
}

// DeviceOSFromID returns the DeviceOS with the given ID.
func DeviceOSFromID(id int) DeviceOS {
	for _, os := range DeviceOSes {
		if os.ID == id {
			return os
		}
	}
	return DeviceOSUnknown
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
