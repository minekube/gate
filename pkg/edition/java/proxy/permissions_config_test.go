package proxy

import (
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/util/permission"
	"go.minekube.com/gate/pkg/util/uuid"
)

// consoleSubject is a permission subject that is not a player.
type consoleSubject struct{}

func (consoleSubject) HasPermission(string) bool                  { return false }
func (consoleSubject) PermissionValue(string) permission.TriState { return permission.Undefined }

// newConfigPermissionsTestProxy returns a Proxy whose event manager is live and
// whose config snapshot holds cfg; no listeners are started.
func newConfigPermissionsTestProxy(cfg *config.Config) *Proxy {
	px := &Proxy{event: event.New()}
	px.currentCfg.Store(&runtimeConfigSnapshot{cfg: cfg})
	return px
}

func firePermissionsSetup(px *Proxy, subject permission.Subject) *PermissionsSetupEvent {
	ev := &PermissionsSetupEvent{
		subject:     subject,
		defaultFunc: func(string) permission.TriState { return permission.Undefined },
	}
	px.event.Fire(ev)
	return ev
}

// TestProxyInitInstallsConfigPermissionsProvider pins the wiring: the provider
// must be registered by Proxy.init(), which runs before the plugin init hooks,
// so no extension is needed and an extension can still override it.
func TestProxyInitInstallsConfigPermissionsProvider(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.RequireBuiltinCommandPermissions = true
	cfg.Permissions = config.Permissions{Default: []string{"gate.command.server"}}

	mgr := event.New()
	p, err := New(Options{Config: &cfg, EventMgr: mgr})
	require.NoError(t, err)
	require.NoError(t, p.init())

	ev := firePermissionsSetup(p, &testPermissionSubject{id: uuid.New(), username: "Player"})
	require.Equal(t, permission.True, ev.Func()("gate.command.server"),
		"Proxy.init must install the config.permissions provider")
	require.Equal(t, permission.Undefined, ev.Func()("gate.command.send"))
}

// TestConfigPermissionsEmptyBlockKeepsTodaysBehaviour pins that an empty
// permissions block registers no provider at all, so enabling
// requireBuiltinCommandPermissions without config values still denies every
// node exactly as before this feature.
func TestConfigPermissionsEmptyBlockKeepsTodaysBehaviour(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.RequireBuiltinCommandPermissions = true
	px := newConfigPermissionsTestProxy(&cfg)
	px.setupConfigPermissions()

	ev := firePermissionsSetup(px, &testPermissionSubject{id: uuid.New(), username: "Player"})
	require.Equal(t, permission.Undefined, ev.Func()("gate.command.server"))
}

// TestConfigPermissionsIgnoresNonPlayerSubjects pins that console and other
// non-player subjects are untouched: the provider is scoped to players.
func TestConfigPermissionsIgnoresNonPlayerSubjects(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.Permissions = config.Permissions{Default: []string{"*"}}
	px := newConfigPermissionsTestProxy(&cfg)
	px.setupConfigPermissions()

	ev := firePermissionsSetup(px, consoleSubject{})
	require.Equal(t, permission.Undefined, ev.Func()("gate.command.server"))
}

// TestConfigPermissionsExtensionOverridesProvider pins the escape hatch: an
// extension that answers PermissionsSetupEvent after the provider was installed
// still replaces it with SetFunc, so existing permission extensions keep
// working and can override a config grant.
func TestConfigPermissionsExtensionOverridesProvider(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.Permissions = config.Permissions{Default: []string{"gate.command.server"}}
	px := newConfigPermissionsTestProxy(&cfg)
	px.setupConfigPermissions()

	event.Subscribe(px.event, 0, func(e *PermissionsSetupEvent) {
		e.SetFunc(func(string) permission.TriState { return permission.False })
	})

	ev := firePermissionsSetup(px, &testPermissionSubject{id: uuid.New(), username: "Player"})
	require.Equal(t, permission.False, ev.Func()("gate.command.server"),
		"an extension answering the event later must be able to override the config provider")
}

// TestConfigPermissionsLeavesUnknownNodesToOtherProviders pins that the provider
// composes with a provider that already answered (a higher priority subscriber)
// instead of hiding it: config nodes win, everything else follows the other
// provider.
func TestConfigPermissionsLeavesUnknownNodesToOtherProviders(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.Permissions = config.Permissions{Default: []string{"gate.command.server"}}
	px := newConfigPermissionsTestProxy(&cfg)
	event.Subscribe(px.event, 5, func(e *PermissionsSetupEvent) {
		e.SetFunc(func(node string) permission.TriState {
			if node == "custom.node" {
				return permission.True
			}
			return permission.False
		})
	})
	px.setupConfigPermissions()

	ev := firePermissionsSetup(px, &testPermissionSubject{id: uuid.New(), username: "Player"})
	require.Equal(t, permission.True, ev.Func()("gate.command.server"), "the config provider decides the nodes it knows")
	require.Equal(t, permission.True, ev.Func()("custom.node"), "nodes the block does not know stay with the other provider")
	require.Equal(t, permission.False, ev.Func()("other.node"), "nodes the block does not know stay with the other provider")
}
