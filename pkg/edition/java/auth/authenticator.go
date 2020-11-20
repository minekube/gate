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
	"go.minekube.com/gate/pkg/edition/java/internal/profile"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/version"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Authenticator can authenticate joining Minecraft clients.
type Authenticator interface {
	// Returns the public key encoded in ASN.1 DER form.
	PublicKey() []byte
	// 1) Verifies the verify token sent by joining client.
	Verify(encryptedVerifyToken, actualVerifyToken []byte) (equal bool, err error)
	// 2) Decrypt shared secret sent by client.
	DecryptSharedSecret(encrypted []byte) (decrypted []byte, err error)
	// 3) Generate server id to be used with AuthenticateJoin.
	GenerateServerID(decryptedSharedSecret []byte) (serverID string, err error)
	// 4) Authenticates a joining user. The ip is optional.
	AuthenticateJoin(ctx context.Context, serverID, username, ip string) (*Response, error)
	// Returns a new Authenticator that uses the specified logger.
	WithLogger(logger logr.Logger) Authenticator
}

// Response is the authentication response.
type Response struct {
	OnlineMode bool   // Whether the user is in online mode
	Body       []byte // The http body the auth server returned
}

// GameProfile extracts the GameProfile from an authenticated user.
func (r *Response) GameProfile() (*profile.GameProfile, error) {
	if r == nil || !r.OnlineMode || len(r.Body) == 0 {
		return nil, errors.New("was not authenticated online mode")
	}
	var p profile.GameProfile
	if err := json.Unmarshal(r.Body, &p); err != nil {
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

const (
	// Default bit size of a generated private key.
	DefaultPrivateKeyBits = 1024
)

var mojangURL = mustURL(url.Parse(
	`https://sessionserver.mojang.com/session/minecraft/hasJoined`))

// Options to create a new Authenticator.
type Options struct {
	// The servers private key.
	// If none is set, a new one will be generated.
	PrivateKey *rsa.PrivateKey
	// PrivateKey is not set,
	// the bit size of a generated private key.
	// The default is DefaultPrivateKeyBits.
	PrivateKeyBits int
	// The http client to query the mojang API.
	// If none is set, a new one is created.
	Client *http.Client
}

// New returns a new basic Mojang user authenticator.
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
		cli = &http.Client{}
	}
	cli.Transport = withHeader(cli.Transport, version.UserAgentHeader())

	return &authn{
		log:     logr.NullLog,
		private: private,
		public:  public,
		cli:     cli,
	}, nil
}

type authn struct {
	log     logr.Logger
	private *rsa.PrivateKey
	public  []byte // ASN.1 DER form encoded
	cli     *http.Client
}

func (a *authn) WithLogger(logger logr.Logger) Authenticator {
	return &authn{
		log:     logger,
		private: a.private,
		public:  a.public,
		cli:     a.cli,
	}
}

func (a *authn) PublicKey() []byte {
	return a.public
}

var _ Authenticator = (*authn)(nil)

func (a *authn) Verify(encryptedVerifyToken, actualVerifyToken []byte) (bool, error) {
	decryptedVerifyToken, err := rsa.DecryptPKCS1v15(rand.Reader, a.private, encryptedVerifyToken)
	if err != nil {
		return false, fmt.Errorf("error descrypt verify token: %v", err)
	}
	return bytes.Equal(decryptedVerifyToken, actualVerifyToken), nil
}

func (a *authn) DecryptSharedSecret(encrypted []byte) (decrypted []byte, err error) {
	return rsa.DecryptPKCS1v15(rand.Reader, a.private, encrypted)
}

func (a *authn) AuthenticateJoin(ctx context.Context, serverID, username, ip string) (*Response, error) {
	u := *mojangURL // copy
	q := u.Query()
	q.Set("username", username)
	q.Set("serverId", serverID)
	log := a.log.V(1).WithName("authenticateJoin").WithValues("username", username)
	if ip != "" {
		q.Set("ip", ip)
		log = log.WithValues("ip", ip)
	}
	urlStr := u.String()
	log = log.WithValues("serverID", serverID, "url", urlStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating authentication request: %w", err)
	}

	start := time.Now()
	log = log.WithName("request")
	log.Info("Sending http request to Mojang sessionserver", "time", start)

	resp, err := a.cli.Do(req)
	if err != nil {
		log.Error(err, "Error with http request", "duration", time.Since(start))
		return nil, fmt.Errorf("error authenticating join with Mojang sessionserver: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	log.Info("Received response from Mojang sessionserver",
		"duration", time.Since(start), "statusCode", resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNoContent:
		log.Info("Mojang could not find user, potentially offline mode")
	default:
		log.Info("Got unexpected status code from Mojang sessionserver",
			"responseBody", string(body), "statusCode", resp.StatusCode)
		return nil, fmt.Errorf("got unexpected status code (%d) from Mojang sessionserver", resp.StatusCode)
	}

	return &Response{
		OnlineMode: resp.StatusCode == http.StatusOK,
		Body:       body,
	}, nil
}

func (a *authn) GenerateServerID(decryptedSharedSecret []byte) (string, error) {
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

func mustURL(s *url.URL, err error) *url.URL {
	if err != nil {
		panic(err)
	}
	return s
}
