package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasJoinedURL(t *testing.T) {
	for _, e := range []struct {
		serverID, username, ip string
		expected               string
	}{
		{serverID: "123456789", username: "Bob", ip: "", expected: defaultHasJoinedEndpoint + "?serverId=123456789&username=Bob"},
		{serverID: "987654321", username: "Alice", ip: "0.0.0.0", expected: defaultHasJoinedEndpoint + "?ip=0.0.0.0&serverId=987654321&username=Alice"},
	} {
		actual := DefaultHasJoinedURL(e.serverID, e.username, e.ip)
		require.Equal(t, e.expected, actual)
	}
}

// parsedPublicKeyBits returns the RSA modulus size of the authenticator's
// DER encoded public key, i.e. what a Minecraft client receives in the
// Encryption Request packet.
func parsedPublicKeyBits(t *testing.T, a Authenticator) int {
	t.Helper()
	pub, err := x509.ParsePKIXPublicKey(a.PublicKey())
	require.NoError(t, err)
	rsaPub, ok := pub.(*rsa.PublicKey)
	require.True(t, ok, "public key must be an RSA key")
	return rsaPub.N.BitLen()
}

// TestNewHonoursPrivateKeyBits is the regression test for
// Options.PrivateKeyBits being declared and documented but never read:
// the option was silently inert and every key was generated at
// DefaultPrivateKeyBits regardless of what the caller asked for.
func TestNewHonoursPrivateKeyBits(t *testing.T) {
	for _, tt := range []struct {
		name string
		bits int
		want int
	}{
		{name: "configured 2048", bits: 2048, want: 2048},
		{name: "configured 1536", bits: 1536, want: 1536},
		{name: "unset falls back to the default", bits: 0, want: DefaultPrivateKeyBits},
		{name: "negative falls back to the default", bits: -512, want: DefaultPrivateKeyBits},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, err := New(Options{PrivateKeyBits: tt.bits})
			require.NoError(t, err)
			require.Equal(t, tt.want, parsedPublicKeyBits(t, a))
		})
	}
}

// TestNewPrefersExplicitPrivateKey documents the precedence: an explicitly
// provided key wins over the requested bit size.
func TestNewPrefersExplicitPrivateKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	a, err := New(Options{PrivateKey: key, PrivateKeyBits: 1024})
	require.NoError(t, err)

	pub, err := x509.ParsePKIXPublicKey(a.PublicKey())
	require.NoError(t, err)
	rsaPub, ok := pub.(*rsa.PublicKey)
	require.True(t, ok)
	require.Equal(t, 0, rsaPub.N.Cmp(key.N), "explicit key must be used as-is")
}

// TestConfiguredKeyBitsCompleteLoginHandshake exercises the parts of the
// login handshake that depend on the key: the client encrypts a shared
// secret and a verify token with the server's public key, the server
// decrypts both and derives the Mojang hasJoined server id from the
// decrypted secret and the very public key it handed out.
func TestConfiguredKeyBitsCompleteLoginHandshake(t *testing.T) {
	const bits = 2048
	a, err := New(Options{PrivateKeyBits: bits})
	require.NoError(t, err)
	require.Equal(t, bits, parsedPublicKeyBits(t, a))

	pub, err := x509.ParsePKIXPublicKey(a.PublicKey())
	require.NoError(t, err)
	rsaPub := pub.(*rsa.PublicKey)

	// Client side of the handshake.
	sharedSecret := make([]byte, 16)
	_, err = rand.Read(sharedSecret)
	require.NoError(t, err)
	verifyToken := make([]byte, 4)
	_, err = rand.Read(verifyToken)
	require.NoError(t, err)
	encryptedSecret, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPub, sharedSecret)
	require.NoError(t, err)
	encryptedToken, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPub, verifyToken)
	require.NoError(t, err)

	// Server side of the handshake.
	decryptedSecret, err := a.DecryptSharedSecret(encryptedSecret)
	require.NoError(t, err)
	require.Equal(t, sharedSecret, decryptedSecret)

	equal, err := a.Verify(encryptedToken, verifyToken)
	require.NoError(t, err)
	require.True(t, equal)

	serverID, err := a.GenerateServerID(decryptedSecret)
	require.NoError(t, err)
	require.Equal(t, expectedServerID(decryptedSecret, a.PublicKey()), serverID)
}

// expectedServerID mirrors the protocol's server id derivation: SHA-1 over the
// decrypted shared secret followed by the public key the server handed out,
// with the two's complement form for hashes whose high bit is set.
func expectedServerID(sharedSecret, publicKeyDER []byte) string {
	h := sha1.New()
	_, _ = h.Write(sharedSecret)
	_, _ = h.Write(publicKeyDER)
	sum := h.Sum(nil)

	var s strings.Builder
	if sum[0]&0x80 == 0x80 {
		carry := true
		for i := len(sum) - 1; i >= 0; i-- {
			sum[i] = ^sum[i]
			if carry {
				carry = sum[i] == 0xff
				sum[i]++
			}
		}
		s.WriteRune('-')
	}
	s.WriteString(strings.TrimLeft(hex.EncodeToString(sum), "0"))
	return s.String()
}
