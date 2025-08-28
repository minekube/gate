package proxy

import (
    "bytes"
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "os"
    "path/filepath"
    "strings"
    "testing"

    jconfig "go.minekube.com/gate/pkg/edition/java/config"
)

// buildFloodgatePayload constructs an encrypted Floodgate string for tests.
func buildFloodgatePayload(t *testing.T, key []byte, bedrockData string) string {
    t.Helper()
    block, err := aes.NewCipher(key)
    if err != nil {
        t.Fatalf("aes: %v", err)
    }
    aead, err := cipher.NewGCM(block)
    if err != nil {
        t.Fatalf("gcm: %v", err)
    }
    iv := make([]byte, 12)
    if _, err := rand.Read(iv); err != nil {
        t.Fatalf("rand: %v", err)
    }
    ct := aead.Seal(nil, iv, []byte(bedrockData), nil)
    encIV := base64.StdEncoding.EncodeToString(iv)
    encCT := base64.StdEncoding.EncodeToString(ct)
    header := fgIdentifier + string(rune(fgVersion+0x3E))
    var b bytes.Buffer
    b.WriteString(header)
    b.WriteString(encIV)
    b.WriteByte(0x21)
    b.WriteString(encCT)
    return b.String()
}

func writeTempKeyFile(t *testing.T, key []byte) string {
    t.Helper()
    dir := t.TempDir()
    p := filepath.Join(dir, "key.pem")
    if err := os.WriteFile(p, key, 0600); err != nil {
        t.Fatalf("write key: %v", err)
    }
    return p
}

func TestFloodgateDetectAndParse(t *testing.T) {
    key := make([]byte, 16)
    if _, err := rand.Read(key); err != nil {
        t.Fatalf("rand key: %v", err)
    }
    // Build minimal BedrockData string per Geyser schema (12 fields)
    bedrock := strings.Join([]string{
        "1.20.0",           // version
        "BedrockUser",      // username
        "1234567890",       // xuid
        "1",                // deviceOs
        "en_US",            // languageCode
        "0",                // uiProfile
        "1",                // inputMode
        "203.0.113.5",      // ip
        "null",             // linkedPlayer (raw)
        "1",                // fromProxy
        "42",               // subscribeId
        "verify"}, "\x00") // verifyCode

    payload := buildFloodgatePayload(t, key, bedrock)
    host := "play.example.com\x00" + payload + "\x00rest"

    cfg := &jconfig.Config{}
    cfg.Floodgate.Enabled = true
    cfg.Floodgate.KeyFile = writeTempKeyFile(t, key)
    cfg.Floodgate.UsernamePrefix = "."
    cfg.Floodgate.ReplaceSpaces = true

    res, err := detectFloodgate(host, cfg)
    if err != nil {
        t.Fatalf("detect err: %v", err)
    }
    if !res.Verified {
        t.Fatalf("expected verified")
    }
    if res.CleanVHost == "" || strings.Contains(res.CleanVHost, fgIdentifier) {
        t.Fatalf("unexpected cleaned host: %q", res.CleanVHost)
    }
    if res.JavaName == "" || !strings.HasPrefix(res.JavaName, cfg.Floodgate.UsernamePrefix) {
        t.Fatalf("unexpected java name: %q", res.JavaName)
    }
}

