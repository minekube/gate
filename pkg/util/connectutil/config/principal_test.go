package config

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.minekube.com/connect/bedrockprincipal"

	"go.minekube.com/gate/pkg/util/connectutil"
)

const (
	testIssuer      = "https://issuer.test"
	testTrustDomain = "urn:minekube:connect:test"
	testAudience    = "urn:minekube:gate:test"
	testKID         = "gate-test-key"
)

func testKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func testPrincipalConfig(pub ed25519.PublicKey) BedrockPrincipal {
	return BedrockPrincipal{
		Mode:        "require",
		Issuer:      testIssuer,
		TrustDomain: testTrustDomain,
		Audience:    testAudience,
		Keys:        map[string]string{testKID: base64.RawURLEncoding.EncodeToString(pub)},
	}
}

func b64u(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// testClaims returns a complete, valid unlinked v2 claim set bound to wire.
func testClaims(wire *connectutil.SessionPrincipalWire, sessionID, jti string) map[string]any {
	now := time.Now().Unix()
	return map[string]any{
		"version":                 2,
		"issuer":                  testIssuer,
		"trust_domain":            testTrustDomain,
		"audience":                testAudience,
		"subject_kind":            "bedrock_xuid",
		"canonical_xuid":          "1",
		"canonical_unlinked_uuid": "00000000-0000-0000-0000-000000000001",
		"bedrock_display_name":    "Sentinel Gamertag",
		"endpoint_id":             wire.EndpointID,
		"organization_id":         wire.OrganizationID,
		"connect_session_id":      sessionID,
		"connect_session_nonce":   b64u(wire.ConnectSessionNonce[:]),
		"policy_revision":         wire.PolicyRevision,
		"source_protocol":         "bedrock",
		"source_protocol_version": wire.SourceProtocolVersion,
		"iat":                     now,
		"nbf":                     now,
		"exp":                     now + 30,
		"jti":                     jti,
		"verification_method":     "minecraft_full_jwks+client_jwt+ecdh_v1",
	}
}

// signEnvelope produces a compact JWS with the frozen wire type. This is a
// test-only signer; Gate itself never signs principals.
func signEnvelope(t *testing.T, priv ed25519.PrivateKey, kid string, claims map[string]any) []byte {
	t.Helper()
	header, err := json.Marshal(map[string]string{
		"alg": "EdDSA",
		"typ": bedrockprincipal.WireType,
		"kid": kid,
	})
	require.NoError(t, err)
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	signingInput := b64u(header) + "." + b64u(payload)
	signature := ed25519.Sign(priv, []byte(signingInput))
	return []byte(signingInput + "." + b64u(signature))
}

func testWire(envelope []byte) *connectutil.SessionPrincipalWire {
	var nonce [16]byte
	copy(nonce[:], "0123456789abcdef")
	return &connectutil.SessionPrincipalWire{
		Protocol:              connectutil.SessionProtocolBedrock,
		EndpointID:            "endpoint-1",
		OrganizationID:        "org-1",
		ConnectSessionNonce:   nonce,
		SourceProtocolVersion: 3,
		PolicyRevision:        7,
		Envelope:              envelope,
	}
}

func testJTI(seed byte) string {
	jti := make([]byte, 16)
	for i := range jti {
		jti[i] = seed
	}
	return b64u(jti)
}

func TestNewPrincipalVerifierOffModes(t *testing.T) {
	for _, mode := range []string{"", "off", "OFF"} {
		p, err := newPrincipalVerifier(BedrockPrincipal{Mode: mode})
		require.NoError(t, err)
		require.Nil(t, p)
		require.Empty(t, p.capabilities())
		require.False(t, p.readiness().Ready())
	}
}

func TestNewPrincipalVerifierRejectsInvalidConfig(t *testing.T) {
	pub, _ := testKeyPair(t)
	for name, mutate := range map[string]func(*BedrockPrincipal){
		"unknown mode":  func(c *BedrockPrincipal) { c.Mode = "warn" },
		"missing trust": func(c *BedrockPrincipal) { c.Issuer = "" },
		"missing keys":  func(c *BedrockPrincipal) { c.Keys = nil },
		"empty kid":     func(c *BedrockPrincipal) { c.Keys = map[string]string{"": c.Keys[testKID]} },
		"bad key":       func(c *BedrockPrincipal) { c.Keys = map[string]string{testKID: "not-a-key"} },
		"padded key": func(c *BedrockPrincipal) {
			c.Keys = map[string]string{testKID: base64.StdEncoding.EncodeToString(pub)}
		},
	} {
		c := testPrincipalConfig(pub)
		mutate(&c)
		_, err := newPrincipalVerifier(c)
		require.Error(t, err, name)
	}
}

func TestPrincipalVerifierReadinessAndCapability(t *testing.T) {
	pub, _ := testKeyPair(t)
	p, err := newPrincipalVerifier(testPrincipalConfig(pub))
	require.NoError(t, err)
	require.NotNil(t, p)
	require.True(t, p.readiness().Ready())
	require.Equal(t, []string{bedrockprincipal.Capability}, p.capabilities())
}

func TestPrincipalVerifierVerifiesUnlinkedPrincipalExactlyOnce(t *testing.T) {
	pub, priv := testKeyPair(t)
	p, err := newPrincipalVerifier(testPrincipalConfig(pub))
	require.NoError(t, err)

	wire := testWire(nil)
	wire.Envelope = signEnvelope(t, priv, testKID, testClaims(wire, "sess-1", testJTI(1)))

	principal, err := p.verify(context.Background(), "sess-1", wire)
	require.NoError(t, err)
	profile := principal.EffectiveGameProfile()
	require.Equal(t, "00000000-0000-0000-0000-000000000001", profile.UUID.String())
	require.Equal(t, "Sentinel Gamertag", profile.Name)
	require.Equal(t, bedrockprincipal.BedrockXUID, principal.SubjectKind())
	_, linked := principal.LinkedJava()
	require.False(t, linked)

	// The replay consumer admits every envelope exactly once.
	_, err = p.verify(context.Background(), "sess-1", wire)
	require.ErrorIs(t, err, bedrockprincipal.Replay)
}

func TestPrincipalVerifierRejectsBindingMismatch(t *testing.T) {
	pub, priv := testKeyPair(t)
	p, err := newPrincipalVerifier(testPrincipalConfig(pub))
	require.NoError(t, err)

	wire := testWire(nil)
	wire.Envelope = signEnvelope(t, priv, testKID, testClaims(wire, "sess-1", testJTI(2)))
	wire.EndpointID = "endpoint-other"

	_, err = p.verify(context.Background(), "sess-1", wire)
	require.ErrorIs(t, err, bedrockprincipal.BindingMismatch)
	require.Equal(t, "BINDING_MISMATCH", principalErrorCategory(err))
}

func TestPrincipalVerifierRejectsNonBedrockProtocol(t *testing.T) {
	pub, priv := testKeyPair(t)
	p, err := newPrincipalVerifier(testPrincipalConfig(pub))
	require.NoError(t, err)

	wire := testWire(nil)
	wire.Envelope = signEnvelope(t, priv, testKID, testClaims(wire, "sess-1", testJTI(3)))
	wire.Protocol = 1 // SESSION_PROTOCOL_JAVA

	_, err = p.verify(context.Background(), "sess-1", wire)
	require.ErrorIs(t, err, bedrockprincipal.BindingMismatch)
}

func TestPrincipalVerifierRejectsUnknownKeyAndTamper(t *testing.T) {
	pub, _ := testKeyPair(t)
	_, otherPriv := testKeyPair(t)
	p, err := newPrincipalVerifier(testPrincipalConfig(pub))
	require.NoError(t, err)

	wire := testWire(nil)
	wire.Envelope = signEnvelope(t, otherPriv, "unknown-kid", testClaims(wire, "sess-1", testJTI(4)))
	_, err = p.verify(context.Background(), "sess-1", wire)
	require.ErrorIs(t, err, bedrockprincipal.Trust)

	wire = testWire(nil)
	wire.Envelope = signEnvelope(t, otherPriv, testKID, testClaims(wire, "sess-1", testJTI(5)))
	_, err = p.verify(context.Background(), "sess-1", wire)
	require.ErrorIs(t, err, bedrockprincipal.Signature)

	wire = testWire([]byte("not-a-jws"))
	_, err = p.verify(context.Background(), "sess-1", wire)
	require.ErrorIs(t, err, bedrockprincipal.Malformed)
}

func TestNilPrincipalVerifierRejectsEnvelopesAsNotReady(t *testing.T) {
	var p *principalVerifier
	wire := testWire([]byte("envelope"))
	_, err := p.verify(context.Background(), "sess-1", wire)
	require.ErrorIs(t, err, bedrockprincipal.Readiness)
	require.Equal(t, "READINESS", principalErrorCategory(err))
}

func TestPrincipalErrorCategoryBoundsUnknownErrors(t *testing.T) {
	require.Equal(t, "INTERNAL", principalErrorCategory(context.Canceled))
}
