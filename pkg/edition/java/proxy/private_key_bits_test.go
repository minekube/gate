package proxy

import (
	"crypto/rsa"
	"crypto/x509"
	"sync/atomic"
	"testing"

	"github.com/go-logr/logr/funcr"

	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
)

func authenticatorKeyBits(t *testing.T, p *Proxy) int {
	t.Helper()
	pub, err := x509.ParsePKIXPublicKey(p.authenticator.PublicKey())
	if err != nil {
		t.Fatalf("parse authenticator public key: %v", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("authenticator public key is %T, want *rsa.PublicKey", pub)
	}
	return rsaPub.N.BitLen()
}

// TestProxyUsesConfiguredPrivateKeyBits covers the config wiring of
// auth.privateKeyBits: the size an operator sets in the config must reach the
// authenticator that generates the login handshake key. Before the wiring, the
// setting was accepted by config.Validate but silently ignored, because
// proxy.New built auth.Options with the session server URL only.
func TestProxyUsesConfiguredPrivateKeyBits(t *testing.T) {
	for _, tt := range []struct {
		name string
		bits int
		want int
	}{
		{name: "unset uses the default", bits: 0, want: auth.DefaultPrivateKeyBits},
		{name: "configured 2048", bits: 2048, want: 2048},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig
			cfg.Bind = "127.0.0.1:0"
			cfg.Auth.PrivateKeyBits = tt.bits

			p, err := New(Options{Config: &cfg})
			if err != nil {
				t.Fatalf("proxy New error: %v", err)
			}
			// Proxy goroutines can outlive the test body, so stop logging with it.
			var testDone atomic.Bool
			defer testDone.Store(true)
			p.log = funcr.New(func(prefix, args string) {
				if !testDone.Load() {
					t.Logf("PROXY: %s %s", prefix, args)
				}
			}, funcr.Options{})

			if got := authenticatorKeyBits(t, p); got != tt.want {
				t.Errorf("login key size = %d bits, want %d", got, tt.want)
			}
		})
	}
}

// TestConfiguredPrivateKeyBitsSurviveInit guards the reload/init path: init()
// re-applies the authenticator settings, and it must not reset the key size
// chosen by the config.
func TestConfiguredPrivateKeyBitsSurviveInit(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.Auth.PrivateKeyBits = 2048
	cfg.Servers = map[string]string{"lobby": "127.0.0.1:1"}
	cfg.Try = []string{"lobby"}

	p, err := New(Options{Config: &cfg})
	if err != nil {
		t.Fatalf("proxy New error: %v", err)
	}
	var testDone atomic.Bool
	defer testDone.Store(true)
	p.log = funcr.New(func(prefix, args string) {
		if !testDone.Load() {
			t.Logf("PROXY: %s %s", prefix, args)
		}
	}, funcr.Options{})

	if err := p.init(); err != nil {
		t.Fatalf("proxy init error: %v", err)
	}
	if got := authenticatorKeyBits(t, p); got != 2048 {
		t.Errorf("login key size after init = %d bits, want 2048", got)
	}
}
