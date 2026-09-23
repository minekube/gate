package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/connect"

	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/connectutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

// TestWrapTunnelSessionExposesVerifiedProfile proves an envelope session's
// connection keeps the verifier-produced game profile visible to
// netmc.Assert, which unwraps via a Conn() net.Conn method (and, for wrappers
// that only embed net.Conn, via Unwrap() net.Conn) and would otherwise
// silently drop the verified profile at login.
func TestWrapTunnelSessionExposesVerifiedProfile(t *testing.T) {
	pub, priv := testKeyPair(t)
	p, err := newPrincipalVerifier(testPrincipalConfig(pub))
	require.NoError(t, err)

	wire := testWire(nil)
	wire.Envelope = signEnvelope(t, priv, testKID, testClaims(wire, "sess-wrap-1", testJTI(9)))
	principal, err := p.verify(context.Background(), "sess-wrap-1", wire)
	require.NoError(t, err)

	verified := principal.EffectiveGameProfile()
	gp := &profile.GameProfile{ID: uuid.UUID(verified.UUID), Name: verified.Name}
	session := &connect.Session{Id: "sess-wrap-1"}

	conn := wrapTunnelSession(nil, session, gp, principal, true)

	gpp, ok := netmc.Assert[proxy.GameProfileProvider](conn)
	require.True(t, ok, "envelope session connection must implement proxy.GameProfileProvider")
	require.Equal(t, gp, gpp.GameProfile())

	pp, ok := netmc.Assert[connectutil.VerifiedPrincipalProvider](conn)
	require.True(t, ok)
	require.Equal(t, principal, pp.VerifiedPrincipal())

	ingress, ok := netmc.Assert[proxy.ConnectTunnelIngress](conn)
	require.True(t, ok, "a verified Connect tunnel must keep its trusted ingress marker")
	require.True(t, ingress.IsConnectTunnelIngress())

	// A verified principal is a proof, not a claim: it must not be published as
	// a vouch for a proposed profile, whatever the endpoint declared.
	_, ok = netmc.Assert[proxy.ConnectAuthenticatedIdentity](conn)
	require.False(t, ok, "a verified-principal session must not carry the vouch claim")

	require.Equal(t, "sess-wrap-1", conn.Session().GetId())
}

func TestWrapTunnelSessionWithoutPrincipal(t *testing.T) {
	session := &connect.Session{Id: "sess-wrap-2"}
	gp := &profile.GameProfile{Name: "SomeName"}

	conn := wrapTunnelSession(nil, session, gp, nil, true)
	gpp, ok := netmc.Assert[proxy.GameProfileProvider](conn)
	require.True(t, ok)
	require.Equal(t, gp, gpp.GameProfile())
	_, ok = netmc.Assert[connectutil.VerifiedPrincipalProvider](conn)
	require.False(t, ok)
	ingress, ok := netmc.Assert[proxy.ConnectTunnelIngress](conn)
	require.True(t, ok, "a proposed-profile Connect tunnel still originates at Connect")
	require.True(t, ingress.IsConnectTunnelIngress())

	// The vouch claim travels with the proposed profile, so it is only visible
	// to the proxy for the wrapper that carries one.
	vouched, ok := netmc.Assert[proxy.ConnectAuthenticatedIdentity](conn)
	require.True(t, ok, "a proposed-profile tunnel must publish whether it vouches")
	require.True(t, vouched.IsConnectAuthenticatedIdentity())

	unvouched := wrapTunnelSession(nil, session, gp, nil, false)
	claim, ok := netmc.Assert[proxy.ConnectAuthenticatedIdentity](unvouched)
	require.True(t, ok, "an endpoint that accepts offline players still publishes the claim")
	require.False(t, claim.IsConnectAuthenticatedIdentity(),
		"an endpoint that accepts offline players vouches for nothing")

	passthrough := wrapTunnelSession(nil, session, nil, nil, true)
	_, ok = netmc.Assert[proxy.GameProfileProvider](passthrough)
	require.False(t, ok)
	_, ok = netmc.Assert[proxy.ConnectAuthenticatedIdentity](passthrough)
	require.False(t, ok, "a session with no supplied profile has nothing to vouch for")
	ingress, ok = netmc.Assert[proxy.ConnectTunnelIngress](passthrough)
	require.True(t, ok, "a passthrough Connect tunnel must not lose its ingress provenance")
	require.True(t, ingress.IsConnectTunnelIngress())
}
