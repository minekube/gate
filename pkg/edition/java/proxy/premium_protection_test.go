package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/internal/identity"
	"go.minekube.com/gate/pkg/util/uuid"
)

func protectionConfig(mode config.PremiumProtectionMode, names ...string) *config.Config {
	cfg := config.DefaultConfig
	cfg.IdentityStore.Enabled = true
	cfg.IdentityStore.PremiumProtection.Mode = mode
	cfg.IdentityStore.PremiumProtection.Names = names
	return &cfg
}

const protectionTestName = "Steve"

// protectionAccounts returns an account that has never been authenticated and
// one that has been.
func protectionAccounts() (offlineID, boundID uuid.UUID, unauthenticated, authenticated identity.Record) {
	offlineID = uuid.OfflinePlayerUUID(protectionTestName)
	boundID = uuid.New()
	unauthenticated = identity.Record{Name: protectionTestName, ActualID: boundID}
	authenticated = identity.Record{Name: protectionTestName, ActualID: boundID, Authenticated: true}
	return
}

// A protected account accepts only an authenticated login, which is one the
// proxy verified with Mojang or one a trusted connection type vouched for.
func TestPremiumProtectionAllows(t *testing.T) {
	offlineID, boundID, unauthenticated, authenticated := protectionAccounts()

	tests := map[string]struct {
		mode     config.PremiumProtectionMode
		names    []string
		account  identity.Record
		resolved uuid.UUID
		source   identity.Source
		want     bool
	}{
		"none protects nothing": {
			mode: config.PremiumProtectionNone, account: authenticated, resolved: offlineID,
			source: identity.SourceOffline, want: true,
		},
		"list refuses the offline login of a listed name": {
			mode: config.PremiumProtectionList, names: []string{protectionTestName},
			account: authenticated, resolved: offlineID, source: identity.SourceOffline, want: false,
		},
		"list refuses the offline login of an unauthenticated listed name": {
			mode: config.PremiumProtectionList, names: []string{protectionTestName},
			account: unauthenticated, resolved: offlineID, source: identity.SourceOffline, want: false,
		},
		"list accepts the authenticated owner": {
			mode: config.PremiumProtectionList, names: []string{protectionTestName},
			account: authenticated, resolved: boundID, source: identity.SourcePremium, want: true,
		},
		"list accepts an authenticated login claiming an unauthenticated listed name": {
			mode: config.PremiumProtectionList, names: []string{protectionTestName},
			account: unauthenticated, resolved: boundID, source: identity.SourcePremium, want: true,
		},
		"list refuses an injected identity": {
			mode: config.PremiumProtectionList, names: []string{protectionTestName},
			account: authenticated, resolved: boundID, source: identity.SourceInjected, want: false,
		},
		"list refuses an injected identity claiming an unauthenticated listed name": {
			mode: config.PremiumProtectionList, names: []string{protectionTestName},
			account: unauthenticated, resolved: uuid.New(), source: identity.SourceInjected, want: false,
		},
		"list ignores names it does not list": {
			mode: config.PremiumProtectionList, names: []string{"Alex"},
			account: authenticated, resolved: offlineID, source: identity.SourceOffline, want: true,
		},
		"list matches names exactly": {
			mode: config.PremiumProtectionList, names: []string{protectionTestName},
			account: authenticated, resolved: offlineID, source: identity.SourceOffline, want: false,
		},
		"list leaves a differently cased name alone": {
			mode: config.PremiumProtectionList, names: []string{"steve"},
			account: authenticated, resolved: offlineID, source: identity.SourceOffline, want: true,
		},
		"list protects an account by its bound UUID": {
			mode: config.PremiumProtectionList, names: []string{boundID.String()},
			account: authenticated, resolved: offlineID, source: identity.SourceOffline, want: false,
		},
		"all leaves a never-authenticated account open": {
			mode:    config.PremiumProtectionAll,
			account: unauthenticated, resolved: offlineID, source: identity.SourceOffline, want: true,
		},
		"all refuses the offline login of an authenticated account": {
			mode:    config.PremiumProtectionAll,
			account: authenticated, resolved: offlineID, source: identity.SourceOffline, want: false,
		},
		"all refuses an injected login of an authenticated account": {
			mode:    config.PremiumProtectionAll,
			account: authenticated, resolved: boundID, source: identity.SourceInjected, want: false,
		},
		"all accepts the authenticated owner": {
			mode:    config.PremiumProtectionAll,
			account: authenticated, resolved: boundID, source: identity.SourcePremium, want: true,
		},
		"list accepts a connect-authenticated identity": {
			mode: config.PremiumProtectionList, names: []string{protectionTestName},
			account: unauthenticated, resolved: boundID,
			source: identity.SourceConnectAuthenticated, want: true,
		},
		"all accepts a connect-authenticated identity": {
			mode:    config.PremiumProtectionAll,
			account: authenticated, resolved: boundID,
			source: identity.SourceConnectAuthenticated, want: true,
		},
		"all refuses an unvouched supplied identity of an authenticated account": {
			mode:    config.PremiumProtectionAll,
			account: authenticated, resolved: boundID, source: identity.SourceInjected, want: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p := newPremiumProtection(protectionConfig(test.mode, test.names...))
			require.Equal(t, test.want, p.allows(protectionTestName, test.resolved, test.account, test.source))
		})
	}
}

// protectPremiumAccounts stays a shorthand for mode: all, and an explicit mode
// wins over it.
func TestPremiumProtectionAlias(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.IdentityStore.ProtectPremiumAccounts = true
	require.Equal(t, config.PremiumProtectionAll, newPremiumProtection(&cfg).mode)

	cfg.IdentityStore.PremiumProtection.Mode = config.PremiumProtectionList
	cfg.IdentityStore.PremiumProtection.Names = []string{protectionTestName}
	require.Equal(t, config.PremiumProtectionList, newPremiumProtection(&cfg).mode)

	cfg.IdentityStore.PremiumProtection.Mode = ""
	cfg.IdentityStore.ProtectPremiumAccounts = false
	require.Equal(t, config.PremiumProtectionNone, newPremiumProtection(&cfg).mode)
}

// Entries are usernames (trimmed, matched exactly) or UUIDs.
func TestPremiumProtectionNames(t *testing.T) {
	id := uuid.New()
	p := newPremiumProtection(protectionConfig(config.PremiumProtectionList,
		"Steve", "  Alex  ", "steve", id.String(), ""))

	require.Contains(t, p.names, "Steve")
	require.Contains(t, p.names, "Alex")
	require.Contains(t, p.names, "steve", "a differently cased name is a different account")
	require.Len(t, p.names, 3)
	require.Contains(t, p.ids, id)
}
