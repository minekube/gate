// Package auth provides a way to authenticate joining online mode players with Mojang's session server.
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/version"
)

// Authenticator is a Mojang user authenticator.
type Authenticator interface {
	// PublicKey returns the public key encoded in ASN.1 DER form.
	PublicKey() []byte
	// Verify verifies the "verify token" sent by joining client.
	Verify(encryptedVerifyToken, actualVerifyToken []byte) (equal bool, err error)
	// DecryptSharedSecret Decrypt shared secret sent by client.
	DecryptSharedSecret(encrypted []byte) (decrypted []byte, err error)
	// GenerateServerID Generate server id to be used with AuthenticateJoin.
	GenerateServerID(decryptedSharedSecret []byte) (serverID string, err error)
	// AuthenticateJoin Authenticates a joining user. The ip is optional.
	AuthenticateJoin(ctx context.Context, serverID, username, ip string) (Response, error)
	// SetHasJoinedURLFn sets the HasJoinedURLFn.
	// If not set, DefaultHasJoinedURL is used.
	SetHasJoinedURLFn(fn HasJoinedURLFn)
}

// Response is the authentication response.
type Response interface {
	OnlineMode() bool // Whether the user is in online mode
	// GameProfile extracts the GameProfile from an authenticated client.
	// Returns nil, nil if OnlineMode is false.
	GameProfile() (*profile.GameProfile, error)
}

const defaultHasJoinedEndpoint = `https://sessionserver.mojang.com/session/minecraft/hasJoined`

var defaultHasJoinedBaseURL, _ = url.Parse(defaultHasJoinedEndpoint)

// DefaultHasJoinedURL returns the default hasJoined URL for the given serverID and username.
// The userIP is optional.
func DefaultHasJoinedURL(serverID, username, userIP string) string {
	return buildHasJoinedURL(defaultHasJoinedBaseURL, serverID, username, userIP)
}

// CustomHasJoinedURL returns a HasJoinedURLFn that uses the given baseURL instead of the default official Mojang API.
func CustomHasJoinedURL(baseURL *url.URL) HasJoinedURLFn {
	if baseURL == nil {
		baseURL = defaultHasJoinedBaseURL
	}
	return func(serverID, username, userIP string) string {
		return buildHasJoinedURL(baseURL, serverID, username, userIP)
	}
}

// buildHasJoinedURL builds the hasJoined URL for the given baseURL.
func buildHasJoinedURL(baseURL *url.URL, serverID, username, userIP string) string {
	query := url.Values{}
	query.Set("serverId", serverID)
	query.Set("username", username)
	if userIP != "" {
		query.Set("ip", userIP)
	}
	return baseURL.ResolveReference(&url.URL{RawQuery: query.Encode()}).String()
}

// HasJoinedURLFn returns the url to authenticate a
// joining online mode user. Note that userIP is optional.
// See DefaultHasJoinedURL for the default implementation.
type HasJoinedURLFn func(serverID, username, userIP string) string

// DefaultPrivateKeyBits is the default bit size of a generated private key.
const DefaultPrivateKeyBits = 1024

// Options to create a new Authenticator.
type Options struct {
	// This setting allows to an authentication url other
	// than the official "hasJoined" Mojang API endpoint.
	// The returned url is used to authenticate a joining
	// online mode user.
	// If not set, DefaultHasJoinedURL is used.
	HasJoinedURLFn HasJoinedURLFn
	// The servers private key.
	// If none is set, a new one will be generated.
	PrivateKey *rsa.PrivateKey
	// PrivateKey is not set,
	// the bit size of a generated private key.
	// The default is DefaultPrivateKeyBits.
	PrivateKeyBits int
	// The http client to query the Mojang API.
	// If none is set, a new one is created.
	Client *http.Client
}

// New returns a new Authenticator.
func New(options Options) (Authenticator, error) {
	var err error
	private := options.PrivateKey
	if private == nil {
		private, err = rsa.GenerateKey(rand.Reader, DefaultPrivateKeyBits)
		if err != nil {
			return nil, fmt.Errorf("error generate private key: %v", err)
		}
	}

	var public []byte
	public, err = x509.MarshalPKIXPublicKey(private.Public())
	if err != nil {
		return nil, fmt.Errorf("error form public key to PKIX, ASN.1 DER: %v", err)
	}

	private.Precompute()

	cli := options.Client
	if cli == nil {
		cli = &http.Client{
			Timeout: time.Second * 10,
		}
	}
	cli.Transport = otelhttp.NewTransport(cli.Transport)
	cli.Transport = withHeader(cli.Transport, version.UserAgentHeader())

	hasJoinedURLFn := options.HasJoinedURLFn
	if hasJoinedURLFn == nil {
		hasJoinedURLFn = DefaultHasJoinedURL
	}

	return &authenticator{
		private:        private,
		public:         public,
		cli:            cli,
		hasJoinedURLFn: hasJoinedURLFn,
	}, nil
}

type authenticator struct {
	private        *rsa.PrivateKey
	public         []byte // ASN.1 DER form encoded
	cli            *http.Client
	hasJoinedURLFn HasJoinedURLFn
}

var _ Authenticator = (*authenticator)(nil)

func (a *authenticator) SetHasJoinedURLFn(fn HasJoinedURLFn) {
	if fn == nil {
		fn = DefaultHasJoinedURL
	}
	a.hasJoinedURLFn = fn
}

func (a *authenticator) PublicKey() []byte {
	return a.public
}

func (a *authenticator) Verify(encryptedVerifyToken, actualVerifyToken []byte) (bool, error) {
	decryptedVerifyToken, err := rsa.DecryptPKCS1v15(rand.Reader, a.private, encryptedVerifyToken)
	if err != nil {
		return false, fmt.Errorf("error descrypt verify token: %v", err)
	}
	return bytes.Equal(decryptedVerifyToken, actualVerifyToken), nil
}

func (a *authenticator) DecryptSharedSecret(encrypted []byte) (decrypted []byte, err error) {
	return rsa.DecryptPKCS1v15(rand.Reader, a.private, encrypted)
}

var tracer = otel.Tracer("java/auth")

func (a *authenticator) AuthenticateJoin(ctx context.Context, serverID, username, ip string) (Response, error) {
	ctx, span := tracer.Start(ctx, "AuthenticateJoin", trace.WithAttributes(
		attribute.String("server.id", serverID),
		attribute.String("user.name", username),
		attribute.String("user.ip", ip),
	))
	defer span.End()

	hasJoinedURL := a.hasJoinedURLFn(serverID, username, ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hasJoinedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating authentication request: %w", err)
	}

	log := logr.FromContextOrDiscard(ctx).V(1).WithName("authnJoin").WithName("request")
	log.Info("authenticating user against sessionserver", "url", hasJoinedURL)

	start := time.Now()
	resp, err := a.cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error authenticating join with Mojang sessionserver: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		// Player has invalid/outdated session auth token and
		// should restart the game or re-login to Mojang.
	case http.StatusNoContent:
		log.Info("sessionserver could not find user, potentially offline mode")
	default:
		return nil, fmt.Errorf("got unexpected status code (%d) from Mojang sessionserver", resp.StatusCode)
	}

	onlineMode := resp.StatusCode == http.StatusOK && len(body) != 0

	log.Info("user authenticated against sessionserver",
		"onlineMode", onlineMode,
		"time", time.Since(start).String(),
		"statusCode", resp.StatusCode)

	return &response{
		onlineMode: onlineMode,
		body:       body,
	}, nil
}

func (a *authenticator) GenerateServerID(decryptedSharedSecret []byte) (string, error) {
	hash, err := func() (hash []byte, err error) {
		h := sha1.New()
		_, err = h.Write(decryptedSharedSecret)
		if err != nil {
			return nil, err
		}
		_, err = h.Write(a.public)
		if err != nil {
			return nil, err
		}
		return h.Sum(nil), nil
	}()
	if err != nil {
		return "", fmt.Errorf("error writing sha1: %v", err)
	}

	var s strings.Builder
	// Check for negative hash
	if (hash[0] & 0x80) == 0x80 {
		hash = twosComplement(hash)
		s.WriteRune('-')
	}
	s.WriteString(strings.TrimLeft(hex.EncodeToString(hash), "0"))
	return s.String(), nil
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

//
//
//
//
//
//

type response struct {
	onlineMode bool

	once sync.Once // unmarshal body once
	body []byte

	gp  *profile.GameProfile
	err error
}

func (r *response) OnlineMode() bool { return r.onlineMode }

func (r *response) GameProfile() (*profile.GameProfile, error) {
	r.once.Do(func() {
		r.gp, r.err = r.gameProfile()
		r.body = nil
	})
	return r.gp, r.err
}

func (r *response) gameProfile() (*profile.GameProfile, error) {
	if r == nil || !r.onlineMode {
		return nil, errors.New("no GameProfile for offline mode user")
	}
	var p profile.GameProfile
	if err := json.Unmarshal(r.body, &p); err != nil {
		return nil, fmt.Errorf("error unmarshal GameProfile: %w", err)
	}
	// Validate
	if p.Name == "" {
		return nil, fmt.Errorf("response body misses username")
	}
	return &p, nil
}

//
//
//
//
//

func withHeader(rt http.RoundTripper, header http.Header) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return headerRoundTripper{Header: header, rt: rt}
}

type headerRoundTripper struct {
	http.Header
	rt http.RoundTripper
}

func (h headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.Header {
		req.Header[k] = v
	}
	return h.rt.RoundTrip(req)
}
