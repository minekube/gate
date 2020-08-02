package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"github.com/valyala/fasthttp"
	"io"
	"net/url"
	"time"
)

// Authenticator is a Minecraft client login authenticator.
type Authenticator struct {
	ServerKey  *rsa.PrivateKey
	PublicKey  []byte // ASN.1 DER form encoded
	HttpClient *fasthttp.Client
}

func NewAuthenticator() *Authenticator {
	serverId := make([]byte, 8)
	_, _ = io.ReadFull(rand.Reader, serverId)

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

func (a *Authenticator) HasJoined(username, optionalUserIp string, serverId string) (statusCode int, body []byte, err error) {
	var ip string
	if len(optionalUserIp) != 0 {
		ip = fmt.Sprintf("&ip=%s", url.QueryEscape(optionalUserIp))
	}
	uri := fmt.Sprintf(mojangHasJoinedUrl,
		url.QueryEscape(username),
		serverId,
		ip,
	)
	return a.HttpClient.Get(nil, uri)
}

func (a *Authenticator) GenerateServerId(sharedSecret []byte) string {
	s := sha1.New()
	s.Write(sharedSecret)
	s.Write(a.PublicKey)
	return hex.EncodeToString(s.Sum(nil))
}

//func twosComplementHexdigest(digest []byte) string {
//	h := sha1.New()
//	h.Write(serverId)
//	h.Write(secret)
//	h.Write(publicKey)
//	hash := h.Sum(nil)
//
//	// Check for negative hashes
//	negative := (hash[0] & 0x80) == 0x80
//	if negative {
//		hash = twosComplement(hash)
//	}
//
//	// Trim away zeroes
//	res := strings.TrimLeft(hex.EncodeToString(hash), "0")
//	if negative {
//		res = "-" + res
//	}
//
//	return res
//}
//
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
