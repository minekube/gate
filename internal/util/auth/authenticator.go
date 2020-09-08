package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"github.com/valyala/fasthttp"
	"net/url"
	"strings"
	"time"
)

// Authenticator is a Minecraft client login authenticator.
type Authenticator struct {
	ServerKey  *rsa.PrivateKey
	PublicKey  []byte // ASN.1 DER form encoded
	HttpClient *fasthttp.Client
}

func NewAuthenticator() *Authenticator {
	serverKey, _ := rsa.GenerateKey(rand.Reader, 1024)
	serverKey.Precompute()
	publicKey, _ := x509.MarshalPKIXPublicKey(serverKey.Public())

	return &Authenticator{
		ServerKey:  serverKey,
		PublicKey:  publicKey,
		HttpClient: NewDefaultHttpClient(),
	}
}

func NewDefaultHttpClient() *fasthttp.Client {
	return &fasthttp.Client{
		Name:         "Minekube Proxy",
		WriteTimeout: time.Second * 3,
		ReadTimeout:  time.Second * 3,
	}
}

const mojangHasJoinedUrl = "https://sessionserver.mojang.com/session/minecraft/hasJoined?username=%s&serverId=%s%s"

func (a *Authenticator) HasJoined(username, optionalUserIP string, serverID string) (statusCode int, body []byte, err error) {
	var ip string
	if len(optionalUserIP) != 0 {
		ip = fmt.Sprintf("&ip=%s", url.QueryEscape(optionalUserIP))
	}
	uri := fmt.Sprintf(mojangHasJoinedUrl,
		url.QueryEscape(username),
		serverID,
		ip,
	)
	return a.HttpClient.Get(nil, uri)
}

func (a *Authenticator) GenerateServerID(sharedSecret []byte) string {
	hash := func() []byte {
		h := sha1.New()
		_, _ = h.Write(sharedSecret)
		_, _ = h.Write(a.PublicKey)
		return h.Sum(nil)
	}()

	var s strings.Builder
	// Check for negative hash
	if (hash[0] & 0x80) == 0x80 {
		hash = twosComplement(hash)
		s.WriteRune('-')
	}
	s.WriteString(strings.TrimLeft(hex.EncodeToString(hash), "0"))
	return s.String()
}

// big endian!
func twosComplement(p []byte) []byte {
	carry := true
	for i := len(p) - 1; i >= 0; i-- {
		p[i] = ^p[i]
		if carry {
			carry = p[i] == 0xff
			p[i]++
		}
	}
	return p
}
