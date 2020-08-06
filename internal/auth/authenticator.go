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
	_, _ = s.Write(sharedSecret)
	_, _ = s.Write(a.PublicKey)
	return hex.EncodeToString(s.Sum(nil))
}
