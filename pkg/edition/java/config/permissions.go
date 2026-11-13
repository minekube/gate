package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"go.minekube.com/gate/pkg/util/permission"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Permissions configures Gate's built-in, file-backed permission provider.
//
// It is the config-only answer to "how do I let players use /server but not
// /send" without writing a Go extension: the provider answers
// proxy.PermissionsSetupEvent for every player at login and leaves any node it
// does not know undefined, so other providers (extensions, plugins) still
// decide those. See https://github.com/minekube/gate/issues/1293.
//
// Semantics:
//   - default: granted to every player that has no entry under players.
//   - players: a listed player REPLACES default, so an entry grants exactly the
//     nodes it lists.
//   - deny: explicit denials always win, over default and over players.
//
// Keys are player UUIDs or, as a convenience only, usernames matched
// case-insensitively (an offline-mode name is not an identity, so prefer UUIDs).
// Values are permission nodes, a trailing-wildcard prefix such as
// gate.command.*, or * for everything.
type Permissions struct {
	// Default is granted to every player without an entry in Players.
	Default []string `yaml:"default,omitempty" json:"default,omitempty"`
	// Players maps a player UUID or username to the nodes granted to it.
	// A listed player replaces Default.
	Players map[string][]string `yaml:"players,omitempty" json:"players,omitempty"`
	// Deny maps a player UUID or username to explicitly denied nodes.
	Deny map[string][]string `yaml:"deny,omitempty" json:"deny,omitempty"`
}

// Empty reports whether no permission is configured at all.
func (p Permissions) Empty() bool {
	return len(p.Default) == 0 && len(p.Players) == 0 && len(p.Deny) == 0
}

// Validate reports every problem in the block, so a typo fails config
// validation at startup instead of silently not matching at runtime.
func (p Permissions) Validate() []error {
	var errs []error
	p.compile(func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) })
	return errs
}

// Build validates and normalizes the block.
//
// A nil set with a nil error means the block is empty: no provider must be
// registered and behaviour stays exactly as before this feature existed.
func (p Permissions) Build() (*PermissionSet, error) {
	if p.Empty() {
		return nil, nil
	}
	var errs []error
	set := p.compile(func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) })
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}
	return set, nil
}

// PermissionSet is a validated and normalized Permissions block.
//
// Entries are keyed by the player identity they select, so a lookup does not
// have to compare keys case-insensitively: "uuid:<canonical>" or
// "name:<lowercase>".
type PermissionSet struct {
	defaultGrants []string
	players       map[string][]string
	deny          map[string][]string
}

// ValueFor returns the tri-state of node for the player identified by id and
// username: True on a matching grant, False on a matching deny and Undefined
// otherwise, leaving nodes this block does not know to other providers.
func (s *PermissionSet) ValueFor(id uuid.UUID, username, node string) permission.TriState {
	if s == nil {
		return permission.Undefined
	}
	// Explicit denials win over every grant.
	if s.matchesAnyEntry(s.deny, id, username, node) {
		return permission.False
	}
	// A listed player REPLACES default, so a listing without a matching node
	// stays undecided rather than falling back to default.
	if grants, listed := s.playerEntry(s.players, id, username); listed {
		if matchNodePatterns(grants, node) {
			return permission.True
		}
		return permission.Undefined
	}
	if matchNodePatterns(s.defaultGrants, node) {
		return permission.True
	}
	return permission.Undefined
}

// playerEntry resolves the grant list of a listed player, preferring the UUID
// entry over a username entry when both exist.
func (s *PermissionSet) playerEntry(entries map[string][]string, id uuid.UUID, username string) ([]string, bool) {
	if nodes, ok := entries[playerUUIDKey(id)]; ok {
		return nodes, true
	}
	if username != "" {
		if nodes, ok := entries[playerNameKey(username)]; ok {
			return nodes, true
		}
	}
	return nil, false
}

// matchesAnyEntry reports whether either identity form of the player is listed
// with a pattern matching node. Denials are a union: an explicit denial must not
// be bypassed by listing the same player under the other form of key.
func (s *PermissionSet) matchesAnyEntry(entries map[string][]string, id uuid.UUID, username, node string) bool {
	if nodes, ok := entries[playerUUIDKey(id)]; ok && matchNodePatterns(nodes, node) {
		return true
	}
	if username != "" {
		if nodes, ok := entries[playerNameKey(username)]; ok && matchNodePatterns(nodes, node) {
			return true
		}
	}
	return false
}

func playerUUIDKey(id uuid.UUID) string { return "uuid:" + id.String() }

func playerNameKey(username string) string { return "name:" + strings.ToLower(username) }

// matchNodePatterns reports whether any of the configured patterns matches node.
// Validation guarantees the only forms are an exact node, a trailing wildcard
// prefix (gate.command.*) and *.
func matchNodePatterns(patterns []string, node string) bool {
	for _, pattern := range patterns {
		switch {
		case pattern == "*":
			return true
		case strings.HasSuffix(pattern, ".*"):
			if strings.HasPrefix(node, strings.TrimSuffix(pattern, "*")) {
				return true
			}
		case pattern == node:
			return true
		}
	}
	return false
}

// nodePattern matches a permission node, a trailing wildcard prefix like
// gate.command.* or * alone. A wildcard anywhere else is a typo, not a pattern.
var nodePattern = regexp.MustCompile(`^(\*|[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*(\.\*)?)$`)

// compile normalizes the block and reports every problem through report, which
// must not be nil. The returned set is only valid when nothing was reported.
func (p Permissions) compile(report func(string, ...any)) *PermissionSet {
	set := &PermissionSet{
		defaultGrants: p.Default,
		players:       make(map[string][]string, len(p.Players)),
		deny:          make(map[string][]string, len(p.Deny)),
	}

	for i, node := range p.Default {
		if !nodePattern.MatchString(node) {
			report("Invalid config.permissions.default[%d] pattern %q: use a node, a trailing wildcard such as gate.command.* or *", i, node)
		}
	}

	entries := func(field string, from map[string][]string, to map[string][]string) {
		seen := make(map[string]string, len(from))
		for key, nodes := range from {
			normalized, ok := permissionEntryKey(key)
			if !ok {
				report("Invalid config.permissions.%s key %q: use a player UUID or a 2-16 character Minecraft username", field, key)
				continue
			}
			if previous, duplicate := seen[normalized]; duplicate {
				report("Duplicate config.permissions.%s entries %q and %q select the same player", field, previous, key)
				continue
			}
			seen[normalized] = key
			for i, node := range nodes {
				if !nodePattern.MatchString(node) {
					report("Invalid config.permissions.%s[%q][%d] pattern %q: use a node, a trailing wildcard such as gate.command.* or *", field, key, i, node)
				}
			}
			to[normalized] = nodes
		}
	}
	entries("players", p.Players, set.players)
	entries("deny", p.Deny, set.deny)
	return set
}

// permissionEntryKey normalizes a players/deny key: a player UUID preferred, a
// Minecraft username accepted case-insensitively as a convenience.
func permissionEntryKey(key string) (string, bool) {
	if id, err := uuid.Parse(key); err == nil {
		return playerUUIDKey(id), true
	}
	if minecraftUsernamePattern.MatchString(key) {
		return playerNameKey(key), true
	}
	return "", false
}

// validatePermissions validates config.permissions.
func validatePermissions(c *Config, e func(string, ...any)) {
	for _, err := range c.Permissions.Validate() {
		// Validate already formats the full message.
		e("%s", err)
	}
}
