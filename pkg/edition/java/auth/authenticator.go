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
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/version"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
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
	AuthenticateJoin(ctx context.Context, serverID, username, ip string) (Response, error)
	// Returns a new Authenticator that uses the specified logger.
	WithLogger(logger logr.Logger) Authenticator
}

// Response is the authentication response.
type Response interface {
	OnlineMode() bool // Whether the user is in online mode
	// Extracts the GameProfile from an authenticated client.
	// Returns nil if OnlineMode is false.
	GameProfile() (*profile.GameProfile, error)
}

//
//
//
//

const defaultHasJoinedEndpoint = `https://sessionserver.mojang.com/session/minecraft/hasJoined`

// DefaultHasJoinedURL is the default implementation of a HasJoinedURLFn.
func DefaultHasJoinedURL(serverID, username, userIP string) string {
	s := new(strings.Builder)
	s.WriteString(defaultHasJoinedEndpoint + "?serverId=")
	s.WriteString(serverID)
	s.WriteString("&username=")
	s.WriteString(username)
	if userIP != "" {
		s.WriteString("&ip=")
		s.WriteString(userIP)
	}
	return s.String()
}

// HasJoinedURLFn returns the url to authenticate a
// joining online mode user. Note that userIP may be empty.
// See DefaultHasJoinedURL for the default implementation.
type HasJoinedURLFn func(serverID, username, userIP string) string

// Default bit size of a generated private key.
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

	hasJoinedURLFn := options.HasJoinedURLFn
	if hasJoinedURLFn == nil {
		hasJoinedURLFn = DefaultHasJoinedURL
	}

	return &authn{
		log:            logr.NopLog,
		private:        private,
		public:         public,
		cli:            cli,
		hasJoinedURLFn: hasJoinedURLFn,
	}, nil
}

type authn struct {
	log            logr.Logger
	private        *rsa.PrivateKey
	public         []byte // ASN.1 DER form encoded
	cli            *http.Client
	hasJoinedURLFn HasJoinedURLFn
}

func (a *authn) WithLogger(logger logr.Logger) Authenticator {
	return &authn{
		log:            logger,
		private:        a.private,
		public:         a.public,
		cli:            a.cli,
		hasJoinedURLFn: a.hasJoinedURLFn,
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

func (a *authn) AuthenticateJoin(ctx context.Context, serverID, username, ip string) (Response, error) {
	hasJoinedURL := a.hasJoinedURLFn(serverID, username, ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hasJoinedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating authentication request: %w", err)
	}

	log := a.log.V(1).WithName("authnJoin").WithName("request")
	log.Info("Sending http request to Mojang sessionserver", "url", hasJoinedURL)

	start := time.Now()
	resp, err := a.cli.Do(req)
	if err != nil {
		log.Error(err, "Error with http request", "time", time.Since(start).String())
		return nil, fmt.Errorf("error authenticating join with Mojang sessionserver: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

	onlineMode := resp.StatusCode == http.StatusOK && len(body) != 0

	log.Info("User was tested against Mojang sessionserver",
		"onlineMode", onlineMode,
		"time", time.Since(start).String(),
		"statusCode", resp.StatusCode)

	return &response{
		onlineMode: onlineMode,
		body:       body,
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
