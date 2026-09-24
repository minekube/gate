package proxy

import (
	"strings"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/internal/identity"
	"go.minekube.com/gate/pkg/util/uuid"
)

// premiumProtection is the resolved identityStore.premiumProtection policy of
// one configuration: the mode plus the configured names and UUIDs. Usernames
// are matched exactly, like the accounts in the store: names that differ only
// in case are different accounts.
//
// A protected account only accepts an authenticated login. Any other login -
// the username's offline UUID, or an identity a connection type supplied
// without vouching for it - is refused. See allows for what counts as
// authenticated.
type premiumProtection struct {
	mode  config.PremiumProtectionMode
	names map[string]struct{}    // exact usernames
	ids   map[uuid.UUID]struct{} // listed UUIDs
}

func newPremiumProtection(c *config.Config) premiumProtection {
	store := c.IdentityStore
	mode := store.PremiumProtection.Mode
	if mode == "" {
		// protectPremiumAccounts was the only way to ask for "every
		// authenticated account is protected" before the modes existed.
		if store.ProtectPremiumAccounts {
			mode = config.PremiumProtectionAll
		}
	}
	mode = config.NormalizePremiumProtectionMode(mode)

	p := premiumProtection{
		mode:  mode,
		names: make(map[string]struct{}, len(store.PremiumProtection.Names)),
		ids:   make(map[uuid.UUID]struct{}, len(store.PremiumProtection.Names)),
	}
	for _, entry := range store.PremiumProtection.Names {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if id, err := uuid.Parse(entry); err == nil {
			p.ids[id] = struct{}{}
			continue
		}
		p.names[entry] = struct{}{}
	}
	return p
}

// protects reports whether this login must authenticate to use the account.
func (p premiumProtection) protects(name string, resolved uuid.UUID, account identity.Record) bool {
	switch p.mode {
	case config.PremiumProtectionList:
		return p.listed(name, resolved, account)
	case config.PremiumProtectionAll:
		// "all" protects every username that has been logged in with an
		// authenticated identity at least once.
		return account.Authenticated
	default: // none, and any unknown mode Validate already rejected
		return false
	}
}

// listed reports whether the configured list covers this login. A username
// entry covers that name; a UUID entry covers the account bound to it, or a
// login claiming it right now.
func (p premiumProtection) listed(name string, resolved uuid.UUID, account identity.Record) bool {
	if _, ok := p.names[name]; ok {
		return true
	}
	for _, id := range []uuid.UUID{resolved, account.ActualID} {
		if id == uuid.Nil {
			continue
		}
		if _, ok := p.ids[id]; ok {
			return true
		}
	}
	return false
}

// allows reports whether a login may use the account it resolved to.
//
// Whether a login may use a protected account depends on one thing only: was
// this session authenticated? An authenticated login is the authority on who
// holds a username, so it is accepted - and it marks the username as
// authenticated for good. Two sources count as authenticated: the proxy
// verifying the login with Mojang itself, and an identity a Connect endpoint
// that declared it does not accept offline-mode players supplied (source
// connect-authenticated).
//
// Everything else is refused: the username's offline UUID, whatever route
// produced it, and an identity a connection type supplied without vouching for
// it (`injected`, which only means "this UUID is not the username's offline
// UUID"). Usernames that were never authenticated are not protected by mode:
// all, so their owners can still reach them.
func (p premiumProtection) allows(
	name string,
	resolved uuid.UUID,
	account identity.Record,
	source identity.Source,
) bool {
	if !p.protects(name, resolved, account) {
		return true
	}
	return source.Authenticated()
}
