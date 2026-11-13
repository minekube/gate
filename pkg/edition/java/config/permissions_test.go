package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/util/permission"
	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	notchUUID = "069a79f4-44e9-4726-a5be-fca90e38aaf5"
	foxUUID   = "11111111-2222-3333-4444-555555555555"
)

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err)
	return id
}

// documentedPermissions is the block from minekube/gate#1293, verbatim.
const documentedPermissions = `
permissions:
  default:
    - gate.command.server
  players:
    "069a79f4-44e9-4726-a5be-fca90e38aaf5":
      - gate.command.server
      - gate.command.glist
    "Notch":
      - gate.command.*
  deny:
    "Notch":
      - gate.command.send
`

func TestPermissionsYAMLShape(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(documentedPermissions), &cfg))

	require.Equal(t, []string{"gate.command.server"}, cfg.Permissions.Default)
	require.Equal(t, []string{"gate.command.server", "gate.command.glist"}, cfg.Permissions.Players[notchUUID])
	require.Equal(t, []string{"gate.command.*"}, cfg.Permissions.Players["Notch"])
	require.Equal(t, []string{"gate.command.send"}, cfg.Permissions.Deny["Notch"])
	require.Empty(t, cfg.Permissions.Validate())
}

func TestPermissionsValueFor(t *testing.T) {
	perms := Permissions{
		Default: []string{"gate.command.server"},
		Players: map[string][]string{
			notchUUID:     {"gate.command.server", "gate.command.glist"},
			"FoxOf7207":   {"*"},
			"Bob_the_Bot": {"gate.command.glist"},
		},
		Deny: map[string][]string{
			notchUUID:     {"gate.command.send"},
			"bob_the_bot": {"gate.command.glist"},
		},
	}
	set, err := perms.Build()
	require.NoError(t, err)
	require.NotNil(t, set)

	notch := mustUUID(t, notchUUID)
	fox := mustUUID(t, foxUUID)
	stranger := mustUUID(t, "22222222-3333-4444-5555-666666666666")

	tests := []struct {
		name     string
		id       uuid.UUID
		username string
		node     string
		want     permission.TriState
	}{
		{"unlisted player falls back to default", stranger, "Stranger", "gate.command.server", permission.True},
		{"unlisted player is not granted other nodes", stranger, "Stranger", "gate.command.send", permission.Undefined},
		{"exact grant by uuid", notch, "Notch", "gate.command.glist", permission.True},
		{"default node also granted by uuid entry", notch, "Notch", "gate.command.server", permission.True},
		{"deny wins over the uuid entry grant", notch, "Notch", "gate.command.send", permission.False},
		{"unknown node stays undefined", notch, "Notch", "gate.command.other", permission.Undefined},
		{"wildcard star grants everything", fox, "FoxOf7207", "anything.at.all", permission.True},
		{"username key is matched", mustUUID(t, "33333333-4444-5555-6666-777777777777"), "FoxOf7207", "gate.command.send", permission.True},
		{"username key is case-insensitive", mustUUID(t, "33333333-4444-5555-6666-777777777777"), "FOXOF7207", "gate.command.send", permission.True},
		{"deny by username beats the listing", mustUUID(t, "44444444-5555-6666-7777-888888888888"), "Bob_The_Bot", "gate.command.glist", permission.False},
		{"listed player does not inherit default", mustUUID(t, "44444444-5555-6666-7777-888888888888"), "bob_the_bot", "gate.command.server", permission.Undefined},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, set.ValueFor(tt.id, tt.username, tt.node))
		})
	}
}

func TestPermissionsPatterns(t *testing.T) {
	set, err := Permissions{Default: []string{"gate.command.*", "exact.node"}}.Build()
	require.NoError(t, err)

	require.Equal(t, permission.True, set.ValueFor(uuid.Nil, "", "gate.command.server"))
	require.Equal(t, permission.True, set.ValueFor(uuid.Nil, "", "gate.command."), "the wildcard keeps the separating dot")
	require.Equal(t, permission.Undefined, set.ValueFor(uuid.Nil, "", "gate.command"), "gate.command.* does not match the bare prefix")
	require.Equal(t, permission.Undefined, set.ValueFor(uuid.Nil, "", "gate.other.server"))
	require.Equal(t, permission.True, set.ValueFor(uuid.Nil, "", "exact.node"))
	require.Equal(t, permission.Undefined, set.ValueFor(uuid.Nil, "", "exact.node.child"), "an exact node does not grant children")
}

func TestPermissionsEmptyIsNoop(t *testing.T) {
	require.True(t, Permissions{}.Empty())
	set, err := Permissions{}.Build()
	require.NoError(t, err)
	require.Nil(t, set, "an empty block must not install a provider")
	require.Empty(t, Permissions{}.Validate())
	require.Equal(t, permission.Undefined, (*PermissionSet)(nil).ValueFor(uuid.Nil, "", "gate.command.server"))
}

func TestPermissionsValidate(t *testing.T) {
	tests := []struct {
		name        string
		perms       Permissions
		wantMessage string
	}{
		{
			name:        "malformed player key",
			perms:       Permissions{Players: map[string][]string{"Not a player!": {"gate.command.server"}}},
			wantMessage: `Invalid config.permissions.players key "Not a player!"`,
		},
		{
			name:        "empty player key",
			perms:       Permissions{Deny: map[string][]string{"": {"gate.command.server"}}},
			wantMessage: `Invalid config.permissions.deny key ""`,
		},
		{
			name:        "malformed default pattern",
			perms:       Permissions{Default: []string{"gate.*.server"}},
			wantMessage: `Invalid config.permissions.default[0] pattern "gate.*.server"`,
		},
		{
			name:        "pattern with only a trailing wildcard part",
			perms:       Permissions{Default: []string{"*.*"}},
			wantMessage: `Invalid config.permissions.default[0] pattern "*.*"`,
		},
		{
			name:        "malformed grant pattern",
			perms:       Permissions{Players: map[string][]string{notchUUID: {"gate.command*"}}},
			wantMessage: `Invalid config.permissions.players["069a79f4-44e9-4726-a5be-fca90e38aaf5"][0] pattern "gate.command*"`,
		},
		{
			name:        "malformed deny pattern",
			perms:       Permissions{Deny: map[string][]string{"Notch": {""}}},
			wantMessage: `Invalid config.permissions.deny["Notch"][0] pattern ""`,
		},
		{
			name: "duplicate usernames differing in case",
			perms: Permissions{Players: map[string][]string{
				"Notch": {"gate.command.server"},
				"notch": {"gate.command.glist"},
			}},
			wantMessage: "select the same player",
		},
		{
			name: "duplicate uuid spellings",
			perms: Permissions{Players: map[string][]string{
				notchUUID:                          {"gate.command.server"},
				"069a79f444e94726a5befca90e38aaf5": {"gate.command.glist"},
			}},
			wantMessage: "select the same player",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.perms.Validate()
			require.NotEmpty(t, errs)
			var found bool
			for _, err := range errs {
				if strings.Contains(err.Error(), tt.wantMessage) {
					found = true
				}
			}
			require.True(t, found, "want an error containing %q, got %v", tt.wantMessage, errs)
			_, err := tt.perms.Build()
			require.Error(t, err, "Build must refuse an invalid block")
		})
	}
}

func TestPermissionsConfigValidationFailsStartup(t *testing.T) {
	cfg := DefaultConfig
	cfg.Permissions = Permissions{Players: map[string][]string{"Not a player!": {"gate.command.server"}}}

	_, errs := cfg.Validate()
	require.NotEmpty(t, errs)
	var found bool
	for _, err := range errs {
		if strings.Contains(err.Error(), "config.permissions") {
			found = true
		}
	}
	require.True(t, found, "config validation must reject an invalid permissions block, got %v", errs)
}

func TestPermissionsConfigValidationAcceptsValidBlock(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(documentedPermissions), &cfg))
	cfg.Bind = DefaultConfig.Bind
	cfg.Forwarding = DefaultConfig.Forwarding
	cfg.Compression = DefaultConfig.Compression

	_, errs := cfg.Validate()
	require.Empty(t, errs)
}

// TestPermissionsLiteModeWarns pins that a Lite config which sets permissions is
// told the setting is inert: Lite pipes the connection through unchanged, so Gate
// never takes part in login and never applies permissions.
func TestPermissionsLiteModeWarns(t *testing.T) {
	cfg := DefaultConfig
	cfg.Lite = liteconfig.DefaultConfig
	cfg.Lite.Enabled = true
	cfg.Permissions = Permissions{Default: []string{"gate.command.server"}}

	warns, _ := cfg.Validate()
	var found bool
	for _, warn := range warns {
		if strings.Contains(warn.Error(), "Lite mode ignores permissions") {
			found = true
		}
	}
	require.True(t, found, "want a warning that Lite mode ignores permissions, got %v", warns)

	cfg.Permissions = Permissions{}
	warns, _ = cfg.Validate()
	for _, warn := range warns {
		require.NotContains(t, warn.Error(), "Lite mode ignores permissions",
			"an empty permissions block must not warn")
	}
}
