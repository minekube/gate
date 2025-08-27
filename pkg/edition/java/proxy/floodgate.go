package proxy

import (
    "bytes"
    "crypto/aes"
    "crypto/cipher"
    "encoding/base64"
    "errors"
    "fmt"
    "io"
    "net"
    "os"
    "strings"
    "unicode/utf8"

    "go.minekube.com/gate/pkg/edition/java/config"
)

// floodgateCipher implements Floodgate's AES-GCM with Base64 topping and header.
// See GeyserMC Floodgate AesCipher and FloodgateCipher.
type floodgateCipher struct {
    key []byte
}

const (
    fgIdentifier = "^Floodgate^"
    fgVersion    = 0
)

func newFloodgateCipherFromKeyFile(path string) (*floodgateCipher, error) {
    if strings.TrimSpace(path) == "" {
        return nil, errors.New("missing Floodgate key file path")
    }
    key, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read floodgate key: %w", err)
    }
    // Expect raw AES key bytes (length 16 for 128-bit)
    if l := len(key); l != 16 && l != 24 && l != 32 {
        return nil, fmt.Errorf("unexpected AES key length %d (expected 16/24/32)", l)
    }
    return &floodgateCipher{key: key}, nil
}

func (f *floodgateCipher) decryptString(input string) (string, error) {
    // Input is: HEADER + iv + 0x21 + ciphertext; iv and ciphertext are Base64-encoded (topping)
    header := []byte(fgIdentifier + string(rune(fgVersion+0x3E)))
    if len(input) <= len(header) || !strings.HasPrefix(input, string(header)) {
        return "", errors.New("invalid floodgate header")
    }
    rest := []byte(input[len(header):])
    // Locate splitter 0x21 after optional Base64 iv bytes
    idx := bytes.IndexByte(rest, 0x21)
    if idx <= 0 {
        return "", errors.New("invalid floodgate payload format")
    }
    encIV := rest[:idx]
    encCT := rest[idx+1:]
    iv := make([]byte, base64.StdEncoding.DecodedLen(len(encIV)))
    n, err := base64.StdEncoding.Decode(iv, encIV)
    if err != nil {
        return "", fmt.Errorf("decode iv: %w", err)
    }
    iv = iv[:n]
    ct := make([]byte, base64.StdEncoding.DecodedLen(len(encCT)))
    m, err := base64.StdEncoding.Decode(ct, encCT)
    if err != nil {
        return "", fmt.Errorf("decode ciphertext: %w", err)
    }
    ct = ct[:m]
    if len(iv) == 0 || len(ct) == 0 {
        return "", errors.New("empty iv or ciphertext")
    }
    block, err := aes.NewCipher(f.key)
    if err != nil {
        return "", fmt.Errorf("aes: %w", err)
    }
    aead, err := cipher.NewGCM(block)
    if err != nil {
        return "", fmt.Errorf("gcm: %w", err)
    }
    pt, err := aead.Open(nil, iv, ct, nil)
    if err != nil {
        return "", fmt.Errorf("decrypt: %w", err)
    }
    if !utf8.Valid(pt) {
        return "", errors.New("invalid utf8 in plaintext")
    }
    return string(pt), nil
}

// parseFloodgateHostname separates Floodgate blob from hostname (split by \0),
// returning decrypted bedrock data string and cleaned hostname.
func parseFloodgateHostname(hostname string, dec func(string) (string, error)) (data string, cleaned string, ok bool) {
    parts := strings.Split(hostname, "\x00")
    if len(parts) == 1 {
        return "", hostname, false
    }
    var (
        fg string
        b strings.Builder
    )
    for _, p := range parts {
        if fg == "" && strings.HasPrefix(p, fgIdentifier) {
            fg = p
            continue
        }
        if b.Len() > 0 {
            b.WriteByte('\x00')
        }
        io.WriteString(&b, p)
    }
    if fg == "" {
        return "", hostname, false
    }
    plain, err := dec(fg)
    if err != nil {
        return "", hostname, false
    }
    return plain, b.String(), true
}

// bedrockData holds fields parsed from BedrockData string.
type bedrockData struct {
    Version       string
    Username      string
    XUID          string
    DeviceOS      int
    LanguageCode  string
    UIProfile     int
    InputMode     int
    IP            string
    LinkedPlayer  string // raw token; optional
    FromProxy     bool
    SubscribeID   int
    VerifyCode    string
}

func parseBedrockData(s string) (bedrockData, error) {
    // Fields separated by \0, expected length 12 (see Geyser BedrockData)
    var out bedrockData
    parts := strings.Split(s, "\x00")
    if len(parts) != 12 {
        return out, fmt.Errorf("unexpected bedrock data length %d", len(parts))
    }
    out.Version = parts[0]
    out.Username = parts[1]
    out.XUID = parts[2]
    if _, err := fmt.Sscanf(parts[3], "%d", &out.DeviceOS); err != nil {
        return out, err
    }
    out.LanguageCode = parts[4]
    if _, err := fmt.Sscanf(parts[5], "%d", &out.UIProfile); err != nil {
        return out, err
    }
    if _, err := fmt.Sscanf(parts[6], "%d", &out.InputMode); err != nil {
        return out, err
    }
    out.IP = parts[7]
    out.LinkedPlayer = parts[8]
    out.FromProxy = parts[9] == "1"
    if _, err := fmt.Sscanf(parts[10], "%d", &out.SubscribeID); err != nil {
        return out, err
    }
    out.VerifyCode = parts[11]
    return out, nil
}

// buildJavaUsername builds the Java username from Bedrock username and config rules.
func buildJavaUsername(prefix string, replaceSpaces bool, bedrockName string) string {
    name := bedrockName
    if replaceSpaces {
        name = strings.ReplaceAll(name, " ", "_")
    }
    max := 16 - len(prefix)
    if max < 0 {
        max = 0
    }
    if len(name) > max {
        name = name[:max]
    }
    return prefix + name
}

// floodgateTrustResult represents detection result extracted at handshake.
type floodgateTrustResult struct {
    Enabled   bool
    Verified  bool
    CleanVHost string
    JavaName  string
    XUID      string
    RemoteIP  net.Addr
}

func detectFloodgate(hostname string, cfg *config.Config) (floodgateTrustResult, error) {
    res := floodgateTrustResult{}
    if cfg == nil || !cfg.Floodgate.Enabled {
        return res, nil
    }
    fc, err := newFloodgateCipherFromKeyFile(cfg.Floodgate.KeyFile)
    if err != nil {
        return res, err
    }
    dataStr, cleaned, ok := parseFloodgateHostname(hostname, fc.decryptString)
    if !ok {
        return res, nil
    }
    bd, err := parseBedrockData(dataStr)
    if err != nil {
        return res, err
    }
    res.Enabled = true
    res.Verified = true
    res.CleanVHost = cleaned
    res.JavaName = buildJavaUsername(cfg.Floodgate.UsernamePrefix, cfg.Floodgate.ReplaceSpaces, bd.Username)
    res.XUID = bd.XUID
    return res, nil
}

