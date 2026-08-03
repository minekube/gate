package config

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"go.minekube.com/connect/bedrockprincipal"

	"go.minekube.com/gate/pkg/util/connectutil"
)

const (
	bedrockPrincipalModeOff     = "off"
	bedrockPrincipalModeRequire = "require"
)

// principalVerifier wraps the Connect SDK Bedrock principal v2 verifier with
// Gate's trusted configuration and readiness state. A nil *principalVerifier
// means the feature is off: no capability is advertised and any proposal
// carrying an envelope is rejected as not ready rather than downgraded.
type principalVerifier struct {
	issuer      string
	trustDomain string
	audience    string

	verifier        bedrockprincipal.Verifier
	keyCount        int
	selfCheckPassed bool
}

// newPrincipalVerifier validates the configuration and constructs the
// verifier, or returns (nil, nil) when the mode is off.
func newPrincipalVerifier(c BedrockPrincipal) (*principalVerifier, error) {
	switch strings.TrimSpace(strings.ToLower(c.Mode)) {
	case "", bedrockPrincipalModeOff:
		return nil, nil
	case bedrockPrincipalModeRequire:
	default:
		return nil, fmt.Errorf("connect.bedrockPrincipal.mode must be %q or %q, got %q",
			bedrockPrincipalModeOff, bedrockPrincipalModeRequire, c.Mode)
	}
	if c.Issuer == "" || c.TrustDomain == "" || c.Audience == "" {
		return nil, errors.New("connect.bedrockPrincipal requires issuer, trustDomain and audience")
	}
	if len(c.Keys) == 0 {
		return nil, errors.New("connect.bedrockPrincipal requires at least one trusted key")
	}
	keys := make(map[string]ed25519.PublicKey, len(c.Keys))
	for kid, encoded := range c.Keys {
		if kid == "" {
			return nil, errors.New("connect.bedrockPrincipal keys require a non-empty kid")
		}
		key, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || len(key) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("connect.bedrockPrincipal key %q must be an unpadded base64url raw Ed25519 public key", kid)
		}
		keys[kid] = key
	}

	p := &principalVerifier{
		issuer:      c.Issuer,
		trustDomain: c.TrustDomain,
		audience:    c.Audience,
		verifier:    bedrockprincipal.NewVerifier(bedrockprincipal.VerifierConfiguration{Keys: keys}),
		keyCount:    len(keys),
	}
	p.selfCheckPassed = p.selfCheck()
	return p, nil
}

// selfCheck proves the verifier is operational by asserting it returns the
// exact bounded category for a malformed envelope.
func (p *principalVerifier) selfCheck() bool {
	env, err := bedrockprincipal.NewSignedPrincipalEnvelope([]byte("self-check-not-a-principal"))
	if err != nil {
		return false
	}
	_, err = p.verifier.VerifyAndConsume(context.Background(), env, bedrockprincipal.TrustedProposalContext{})
	return errors.Is(err, bedrockprincipal.Malformed)
}

func (p *principalVerifier) readiness() bedrockprincipal.ReadinessState {
	if p == nil {
		return bedrockprincipal.ReadinessState{Mode: bedrockPrincipalModeOff}
	}
	return bedrockprincipal.ReadinessState{
		Mode:                    bedrockPrincipalModeRequire,
		MetadataFresh:           true, // statically configured keys never go stale
		ReplayAvailable:         true,
		ReplayCapacityAvailable: true,
		SelfCheckPassed:         p.selfCheckPassed,
		EligibleKeyCount:        p.keyCount,
	}
}

// capabilities returns the Connect capabilities Gate may advertise right now.
// Advertising is gated on full readiness so a degraded verifier downgrades to
// no advertisement instead of accepting unverifiable Bedrock sessions.
func (p *principalVerifier) capabilities() []string {
	if p != nil && p.readiness().Ready() {
		return []string{bedrockprincipal.Capability}
	}
	return nil
}

// verify consumes the proposal's envelope exactly once against the trusted
// context assembled from configuration and the authenticated session fields.
func (p *principalVerifier) verify(
	ctx context.Context,
	sessionID string,
	wire *connectutil.SessionPrincipalWire,
) (bedrockprincipal.VerifiedBedrockPrincipal, error) {
	if p == nil || !p.readiness().Ready() {
		return nil, bedrockprincipal.Readiness
	}
	if !wire.IsBedrock() {
		return nil, bedrockprincipal.BindingMismatch
	}
	env, err := bedrockprincipal.NewSignedPrincipalEnvelope(wire.Envelope)
	if err != nil {
		return nil, err
	}
	return p.verifier.VerifyAndConsume(ctx, env, bedrockprincipal.TrustedProposalContext{
		Issuer:                p.issuer,
		TrustDomain:           p.trustDomain,
		Audience:              p.audience,
		EndpointID:            wire.EndpointID,
		OrganizationID:        wire.OrganizationID,
		ConnectSessionID:      sessionID,
		ConnectSessionNonce:   wire.ConnectSessionNonce,
		SourceProtocol:        "bedrock",
		SourceProtocolVersion: wire.SourceProtocolVersion,
		PolicyRevision:        wire.PolicyRevision,
	})
}

// principalErrorCategory maps any verification error to the bounded public
// category string; it never carries envelope or identity material.
func principalErrorCategory(err error) string {
	var category bedrockprincipal.PrincipalError
	if errors.As(err, &category) {
		return string(category)
	}
	return string(bedrockprincipal.Internal)
}
