package proxy

import (
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/internal/identity"
	"go.minekube.com/gate/pkg/util/uuid"
)

type identityTestConfig struct{ cfg *config.Config }

func (c identityTestConfig) config() *config.Config { return c.cfg }

func newIdentityTestHandler(
	t *testing.T,
	store *identity.Store,
	cfg *config.Config,
) (*authSessionHandler, *testMinecraftConn) {
	t.Helper()
	mc := &testMinecraftConn{}
	inbound := newTestLoginInboundConn(mc)
	handler := &authSessionHandler{
		sessionHandlerDeps: &sessionHandlerDeps{
			proxy:          &Proxy{identityStore: store},
			configProvider: identityTestConfig{cfg: cfg},
		},
		log:     logr.Discard(),
		inbound: inbound,
	}
	return handler, mc
}

func testIdentityConfig(storeEnabled bool) *config.Config {
	cfg := config.DefaultConfig
	cfg.IdentityStore.Enabled = storeEnabled
	return &cfg
}

func openIdentityStore(t *testing.T, path string) *identity.Store {
	t.Helper()
	store, err := identity.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// The whole point of the store: a login that resolves to a different identity
// than the account's first login is assigned the stored UUID, so both login
// paths end up as one player on the backend.
func TestIdentityStoreAssignsStoredUUID(t *testing.T) {
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	cfg := testIdentityConfig(true)
	offlineID := uuid.OfflinePlayerUUID("Steve")
	premiumID := uuid.New()

	first, _ := newIdentityTestHandler(t, store, cfg)
	profileFirst := &profile.GameProfile{ID: offlineID, Name: "Steve"}
	source, ok := first.assignStoredIdentity(store, profileFirst)
	require.True(t, ok)
	require.Equal(t, identity.SourceOffline, source)
	require.Equal(t, offlineID, profileFirst.ID)

	second, _ := newIdentityTestHandler(t, store, cfg)
	second.onlineMode = true // a Mojang-authenticated login
	profileSecond := &profile.GameProfile{ID: premiumID, Name: "Steve"}
	source, ok = second.assignStoredIdentity(store, profileSecond)

	require.True(t, ok)
	require.Equal(t, identity.SourcePremium, source, "the login itself is authenticated")
	require.Equal(t, offlineID, profileSecond.ID,
		"an authenticated login inherits the identity registered by the offline login")
}

// A store that cannot be read must reject the login by default: continuing
// would hand the player the very UUID the store exists to unify.
func TestIdentityStoreFailsClosed(t *testing.T) {
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, store.Close())

	handler, mc := newIdentityTestHandler(t, store, testIdentityConfig(true))
	gameProfile := &profile.GameProfile{ID: uuid.New(), Name: "Steve"}

	_, ok := handler.assignStoredIdentity(store, gameProfile)

	require.False(t, ok, "the login is rejected")
	require.NotEmpty(t, mc.writtenPackets)
	_, disconnected := mc.writtenPackets[len(mc.writtenPackets)-1].(*packet.Disconnect)
	require.True(t, disconnected, "the player is disconnected with a reason")
}

// With identityStore.failOpen the login continues on the resolved UUID instead.
func TestIdentityStoreFailOpen(t *testing.T) {
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, store.Close())

	cfg := testIdentityConfig(true)
	cfg.IdentityStore.FailOpen = true
	handler, _ := newIdentityTestHandler(t, store, cfg)
	resolved := uuid.New()
	gameProfile := &profile.GameProfile{ID: resolved, Name: "Steve"}

	source, ok := handler.assignStoredIdentity(store, gameProfile)

	require.True(t, ok)
	require.Equal(t, resolved, gameProfile.ID, "the resolved UUID is kept as-is")
	require.Equal(t, identity.SourceInjected, source, "online mode is off in this handler")
}

// An offline-mode identity is recognized by its UUID being the digest of the
// username; anything else was supplied by an authenticated login or a
// connection type such as Geyser or a Connect tunnel.
func TestResolvedIdentitySource(t *testing.T) {
	const name = "Steve"
	unvouched := unvouchedSuppliedIdentity
	vouched := vouchedSuppliedIdentity
	require.Equal(t, identity.SourcePremium,
		resolvedIdentitySource(true, unvouched, profile.GameProfile{ID: uuid.New(), Name: name}))
	require.Equal(t, identity.SourceOffline,
		resolvedIdentitySource(false, unvouched, profile.GameProfile{ID: uuid.OfflinePlayerUUID(name), Name: name}))
	require.Equal(t, identity.SourceInjected,
		resolvedIdentitySource(false, unvouched, profile.GameProfile{ID: uuid.New(), Name: name}))
	require.Equal(t, identity.SourceInjected,
		resolvedIdentitySource(false, unvouched, profile.GameProfile{ID: uuid.OfflinePlayerUUID("Other"), Name: name}),
		"a supplied UUID that is not this name's digest is not an offline identity")

	// A vouch is what makes a supplied identity authenticated, and it is the
	// only thing that does.
	require.Equal(t, identity.SourceConnectAuthenticated,
		resolvedIdentitySource(false, vouched, profile.GameProfile{ID: uuid.New(), Name: name}))
	require.Equal(t, identity.SourcePremium,
		resolvedIdentitySource(true, vouched, profile.GameProfile{ID: uuid.New(), Name: name}),
		"a login the proxy authenticated itself stays premium, not connect-authenticated")

	// The offline UUID is refused whatever the ingress declares, so an endpoint
	// that vouches for a cracked identity still cannot authenticate it.
	require.Equal(t, identity.SourceOffline,
		resolvedIdentitySource(false, vouched, profile.GameProfile{ID: uuid.OfflinePlayerUUID(name), Name: name}),
		"a vouch never turns the username's offline UUID into an authenticated identity")
}

// vouchingIdentityTestConn is a connection whose ingress vouches for the
// identity it supplies. Like Gate's tunnel adapter it declares the marker on a
// connection that also supplies a profile.
type vouchingIdentityTestConn struct {
	*testMinecraftConn
}

func (*vouchingIdentityTestConn) IsConnectAuthenticatedIdentity() bool { return true }

func (*vouchingIdentityTestConn) GameProfile() *profile.GameProfile {
	return &profile.GameProfile{Name: "Steve"}
}

// The connection's declaration and the decision are separate places, so this
// walks the whole seam the way a Connect login does: the vouch is read off the
// connection, and the source it produces is what decides a protected account.
func TestConnectVouchReachesTheProtectionDecision(t *testing.T) {
	const name = "Steve"
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	cfg := testIdentityConfig(true)
	cfg.OnlineMode = true
	cfg.IdentityStore.PremiumProtection.Mode = config.PremiumProtectionAll

	// The account authenticated once and is therefore protected by mode: all.
	owner, _ := newIdentityTestHandler(t, store, cfg)
	owner.onlineMode = true
	_, ok := owner.assignStoredIdentity(store, &profile.GameProfile{ID: uuid.New(), Name: name})
	require.True(t, ok)

	// A connection that vouches is admitted as connect-authenticated.
	vouching, _ := newIdentityTestHandler(t, store, cfg)
	vouching.supplied = suppliedIdentityTrustOf(&vouchingIdentityTestConn{testMinecraftConn: &testMinecraftConn{}})
	require.Equal(t, vouchedSuppliedIdentity, vouching.supplied)
	source, ok := vouching.assignStoredIdentity(store, &profile.GameProfile{ID: uuid.New(), Name: name})
	require.True(t, ok, "a vouched Connect identity may use the protected account")
	require.Equal(t, identity.SourceConnectAuthenticated, source)

	// A connection that does not vouch supplies an unvouched identity, and the
	// same protected account refuses it.
	unvouching, mc := newIdentityTestHandler(t, store, cfg)
	unvouching.supplied = suppliedIdentityTrustOf(&testMinecraftConn{})
	require.Equal(t, unvouchedSuppliedIdentity, unvouching.supplied)
	source, ok = unvouching.assignStoredIdentity(store, &profile.GameProfile{ID: uuid.New(), Name: name})
	require.False(t, ok, "an unvouched Connect identity may not use a protected account")
	require.Equal(t, identity.SourceInjected, source)
	require.IsType(t, &packet.Disconnect{}, mc.writtenPackets[len(mc.writtenPackets)-1])
}

// The vouch is the connection's to make, so a connection that does not make it
// stays unvouched: that is a session from an endpoint that accepts offline-mode
// players, or one that supplied no profile at all.
func TestSuppliedIdentityTrustFollowsTheConnection(t *testing.T) {
	require.Equal(t, vouchedSuppliedIdentity,
		suppliedIdentityTrustOf(&vouchingIdentityTestConn{testMinecraftConn: &testMinecraftConn{}}))

	require.Equal(t, unvouchedSuppliedIdentity,
		suppliedIdentityTrustOf(&testMinecraftConn{}),
		"a connection that does not vouch stays unvouched")
}

// A connect-authenticated login reaches a protected account instead of being
// refused, and is recorded as authenticated, so the protection is sticky from
// then on and the username cannot be taken over by the offline path.
func TestConnectAuthenticatedLoginPassesPremiumProtection(t *testing.T) {
	const name = "Steve"
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	cfg := testIdentityConfig(true)
	cfg.IdentityStore.PremiumProtection.Mode = config.PremiumProtectionAll
	offlineID := uuid.OfflinePlayerUUID(name)
	mojangID := uuid.New()

	tunnel, _ := newIdentityTestHandler(t, store, cfg)
	tunnel.supplied = vouchedSuppliedIdentity
	profileTunnel := &profile.GameProfile{ID: mojangID, Name: name}
	source, ok := tunnel.assignStoredIdentity(store, profileTunnel)
	require.True(t, ok, "a vouched identity is admitted by premium protection")
	require.Equal(t, identity.SourceConnectAuthenticated, source)
	require.Equal(t, mojangID, profileTunnel.ID, "it keeps the UUID the endpoint supplied")

	cracked, mc := newIdentityTestHandler(t, store, cfg)
	_, ok = cracked.assignStoredIdentity(store, &profile.GameProfile{ID: offlineID, Name: name})
	require.False(t, ok, "the account is protected once a vouched login authenticated it")
	require.IsType(t, &packet.Disconnect{}, mc.writtenPackets[len(mc.writtenPackets)-1])
}

// An endpoint that accepts offline-mode players also proposes unauthenticated
// identities, so nothing it supplies is vouched for: the login is refused
// exactly like any other injected identity. This is connect.allowOfflineModePlayers
// left at true.
func TestConnectAuthenticatedLoginRequiresEndpointVouch(t *testing.T) {
	const name = "Steve"
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	cfg := testIdentityConfig(true)
	cfg.IdentityStore.PremiumProtection.Mode = config.PremiumProtectionAll

	// An authenticated login claims and protects the account.
	owner, _ := newIdentityTestHandler(t, store, cfg)
	owner.onlineMode = true
	_, ok := owner.assignStoredIdentity(store, &profile.GameProfile{ID: uuid.New(), Name: name})
	require.True(t, ok)

	unvouched, mc := newIdentityTestHandler(t, store, cfg)
	unvouched.supplied = unvouchedSuppliedIdentity
	_, ok = unvouched.assignStoredIdentity(store, &profile.GameProfile{ID: uuid.New(), Name: name})
	require.False(t, ok, "an unvouched supplied identity may not use a protected account")
	require.IsType(t, &packet.Disconnect{}, mc.writtenPackets[len(mc.writtenPackets)-1])
}

// The offline UUID is the local backstop: it is refused even for a vouch, so an
// endpoint that vouches for a cracked identity still cannot authenticate it.
func TestConnectAuthenticatedLoginRefusesOfflineUUID(t *testing.T) {
	const name = "Steve"
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	cfg := testIdentityConfig(true)
	cfg.IdentityStore.PremiumProtection.Mode = config.PremiumProtectionList
	cfg.IdentityStore.PremiumProtection.Names = []string{name}

	vouched, mc := newIdentityTestHandler(t, store, cfg)
	vouched.supplied = vouchedSuppliedIdentity
	profileVouched := &profile.GameProfile{ID: uuid.OfflinePlayerUUID(name), Name: name}
	source, ok := vouched.assignStoredIdentity(store, profileVouched)
	require.False(t, ok, "the username's offline UUID is refused even when vouched for")
	require.Equal(t, identity.SourceOffline, source)
	require.IsType(t, &packet.Disconnect{}, mc.writtenPackets[len(mc.writtenPackets)-1])
}

// Once an account has been authenticated, an offline login may no longer use
// its username: the protection is sticky from the first authenticated login on.
func TestProtectPremiumAccountsRefusesOfflineLogin(t *testing.T) {
	const name = "Steve"
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	cfg := testIdentityConfig(true)
	cfg.IdentityStore.ProtectPremiumAccounts = true
	offlineID := uuid.OfflinePlayerUUID(name)
	premiumID := uuid.New()

	// An unverified account may still be registered by an offline login.
	first, _ := newIdentityTestHandler(t, store, cfg)
	profileFirst := &profile.GameProfile{ID: offlineID, Name: name}
	source, ok := first.assignStoredIdentity(store, profileFirst)
	require.True(t, ok, "an account without an authenticated login stays open")
	require.Equal(t, identity.SourceOffline, source)

	// The owner logs in with an authenticated identity once.
	second, _ := newIdentityTestHandler(t, store, cfg)
	second.onlineMode = true
	profileSecond := &profile.GameProfile{ID: premiumID, Name: name}
	source, ok = second.assignStoredIdentity(store, profileSecond)
	require.True(t, ok, "the authenticated login is allowed")
	require.Equal(t, identity.SourcePremium, source)
	require.Equal(t, offlineID, profileSecond.ID, "and still inherits the account's stored UUID")

	// From now on the username is authenticated-only, for offline logins.
	third, mc := newIdentityTestHandler(t, store, cfg)
	profileThird := &profile.GameProfile{ID: offlineID, Name: name}
	_, ok = third.assignStoredIdentity(store, profileThird)
	require.False(t, ok, "an offline login for an authenticated username is refused")
	require.IsType(t, &packet.Disconnect{}, mc.writtenPackets[len(mc.writtenPackets)-1],
		"the player is disconnected with a reason")

	// The authenticated owner keeps getting in.
	fourth, _ := newIdentityTestHandler(t, store, cfg)
	fourth.onlineMode = true
	profileFourth := &profile.GameProfile{ID: premiumID, Name: name}
	_, ok = fourth.assignStoredIdentity(store, profileFourth)
	require.True(t, ok, "the authenticated login is still allowed")
}

// Without the option an authenticated account still accepts offline logins, as
// the store's first-use rule has always allowed.
func TestProtectPremiumAccountsDisabledByDefault(t *testing.T) {
	const name = "Steve"
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	cfg := testIdentityConfig(true) // ProtectPremiumAccounts stays false
	offlineID := uuid.OfflinePlayerUUID(name)

	verified, _ := newIdentityTestHandler(t, store, cfg)
	verified.onlineMode = true
	_, ok := verified.assignStoredIdentity(store, &profile.GameProfile{ID: uuid.New(), Name: name})
	require.True(t, ok)

	offline, _ := newIdentityTestHandler(t, store, cfg)
	_, ok = offline.assignStoredIdentity(store, &profile.GameProfile{ID: offlineID, Name: name})
	require.True(t, ok, "without protectPremiumAccounts the offline login is allowed")
}

// The same protection through the mode-based configuration, end to end through
// the login seam: a listed name refuses the cracked path, accepts the owner,
// and accepts an authenticated identity that holds the stored UUID.
func TestPremiumProtectionListRefusesCrackedLogin(t *testing.T) {
	const name = "Steve"
	store := openIdentityStore(t, filepath.Join(t.TempDir(), "identities.db"))
	cfg := testIdentityConfig(true)
	cfg.IdentityStore.PremiumProtection.Mode = config.PremiumProtectionList
	cfg.IdentityStore.PremiumProtection.Names = []string{name}
	offlineID := uuid.OfflinePlayerUUID(name)
	premiumID := uuid.New()

	// The cracked login is refused even though the name was never claimed.
	cracked, mc := newIdentityTestHandler(t, store, cfg)
	_, ok := cracked.assignStoredIdentity(store, &profile.GameProfile{ID: offlineID, Name: name})
	require.False(t, ok, "a listed name refuses the offline login")
	require.IsType(t, &packet.Disconnect{}, mc.writtenPackets[len(mc.writtenPackets)-1])

	// The owner's authenticated login claims it.
	owner, _ := newIdentityTestHandler(t, store, cfg)
	owner.onlineMode = true
	_, ok = owner.assignStoredIdentity(store, &profile.GameProfile{ID: premiumID, Name: name})
	require.True(t, ok, "the authenticated owner is allowed")

	// From now on only an authenticated login may use it, even with the UUID it
	// authenticated with.
	relogin, _ := newIdentityTestHandler(t, store, cfg)
	relogin.onlineMode = true
	_, ok = relogin.assignStoredIdentity(store, &profile.GameProfile{ID: premiumID, Name: name})
	require.True(t, ok, "the authenticated owner is allowed")

	again, _ := newIdentityTestHandler(t, store, cfg)
	_, ok = again.assignStoredIdentity(store, &profile.GameProfile{ID: offlineID, Name: name})
	require.False(t, ok, "the cracked path stays refused after the account is claimed")

	other, _ := newIdentityTestHandler(t, store, cfg)
	_, ok = other.assignStoredIdentity(store, &profile.GameProfile{ID: offlineID, Name: "Alex"})
	require.True(t, ok, "names outside the list are unaffected")
}
