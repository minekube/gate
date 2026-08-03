package config

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/connect"
	"go.minekube.com/connect/bedrockprincipal"
)

// connHandlerFunc adapts a func to the ConnHandler interface.
type connHandlerFunc func(net.Conn)

func (f connHandlerFunc) HandleConn(conn net.Conn) { f(conn) }

// dialCapabilityHeader boots the real Watch client runnable against a local
// fake WatchService and returns the Connect capability headers it received on
// the websocket handshake, exactly as the production WatchService would.
func dialCapabilityHeader(t *testing.T, c Config) (capabilities []string, connector []string) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	captured := make(chan http.Header, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case captured <- r.Header.Clone():
		default:
		}
		cancel() // one handshake is enough; stop the retry loop
		http.Error(w, "test server rejects upgrade", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c.WatchServiceAddr = "ws://" + strings.TrimPrefix(srv.URL, "http://")
	c.Name = "capability-e2e-test" // must be set: empty name triggers an internet lookup
	c.TokenFilePath = filepath.Join(t.TempDir(), "token.json")

	runnable, err := connectClient(c, connHandlerFunc(func(net.Conn) {}))
	require.NoError(t, err)
	_ = runnable.Start(ctx) // returns once ctx is canceled by the handler

	select {
	case h := <-captured:
		caps := h.Values(http.CanonicalHeaderKey(connect.MDPrefix + "capabilities"))
		conn := h.Values(http.CanonicalHeaderKey(connect.MDPrefix + "connector"))
		t.Logf("fake WatchService received handshake: %s=%q %s=%q",
			connect.MDPrefix+"capabilities", caps, connect.MDPrefix+"connector", conn)
		return caps, conn
	default:
		t.Fatal("fake watch service received no handshake")
		return nil, nil
	}
}

func TestWatchDialAdvertisesCapabilityOnlyWhenVerifierReady(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	require_ := Config{BedrockPrincipal: BedrockPrincipal{
		Mode:        "require",
		Issuer:      "https://issuer.test",
		TrustDomain: "test-domain",
		Audience:    "test-audience",
		Keys:        map[string]string{"k1": base64.RawURLEncoding.EncodeToString(pub)},
	}}
	caps, connector := dialCapabilityHeader(t, require_)
	require.Equal(t, []string{bedrockprincipal.Capability}, caps,
		"ready require-mode verifier must advertise the capability on the Watch handshake")
	require.Equal(t, []string{"gate"}, connector)

	caps, _ = dialCapabilityHeader(t, Config{}) // default: bedrockPrincipal off
	require.Empty(t, caps,
		"off/not-ready verifier must not advertise the capability")
}
