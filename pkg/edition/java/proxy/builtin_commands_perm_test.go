package proxy

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/util/uuid"

	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/permission"
)

// testPermissionSubject is the smallest thing the file-backed provider needs:
// a permission subject plus the player identity config.permissions keys use.
type testPermissionSubject struct {
	id       uuid.UUID
	username string
}

func (s *testPermissionSubject) HasPermission(node string) bool { return false }

func (s *testPermissionSubject) PermissionValue(string) permission.TriState {
	return permission.Undefined
}

func (s *testPermissionSubject) ID() uuid.UUID    { return s.id }
func (s *testPermissionSubject) Username() string { return s.username }

// resolvePlayerPermissions runs the real PermissionsSetupEvent delivery for one
// player and returns the permission func the player ends up with.
func resolvePlayerPermissions(t *testing.T, cfg *config.Config, id uuid.UUID, username string) permission.Func {
	t.Helper()
	px := &Proxy{event: event.New()}
	px.currentCfg.Store(&runtimeConfigSnapshot{cfg: cfg})
	px.setupConfigPermissions()

	ev := &PermissionsSetupEvent{
		subject:     &testPermissionSubject{id: id, username: username},
		defaultFunc: func(string) permission.TriState { return permission.Undefined },
	}
	px.event.Fire(ev)
	return ev.Func()
}

// newBuiltinPermFixture wires a player whose permissions come from the real
// config.permissions provider to a command manager holding stand-ins for the
// builtin commands (same permission requirements, recording executors).
func newBuiltinPermFixture(t *testing.T, cfg *config.Config, permFn permission.Func) (*connectedPlayer, *recordingConn, *command.Manager, *[]string) {
	t.Helper()
	conn := &recordingConn{
		testMinecraftConn: &testMinecraftConn{},
		protocol:          version.Minecraft_1_21_5.Protocol,
	}
	player := &connectedPlayer{
		MinecraftConn:      conn,
		sessionHandlerDeps: &sessionHandlerDeps{configProvider: fakeConfigProvider{cfg: cfg}},
		log:                logr.Discard(),
		profile:            &profile.GameProfile{ID: uuid.New(), Name: "Player"},
		permFunc:           permFn,
	}

	px := &Proxy{event: event.New()}
	px.currentCfg.Store(&runtimeConfigSnapshot{cfg: cfg})

	var ran []string
	cmdMgr := &command.Manager{}
	cmdMgr.Register(brigodier.Literal("server").
		Requires(hasCmdPerm(px, serverCmdPermission)).
		Executes(command.Command(func(*command.Context) error {
			ran = append(ran, "server")
			return nil
		})))
	cmdMgr.Register(brigodier.Literal("send").
		Requires(hasCmdPerm(px, sendCmdPermission)).
		Then(brigodier.Argument("player", brigodier.String).
			Executes(command.Command(func(*command.Context) error {
				ran = append(ran, "send")
				return nil
			}))))
	return player, conn, cmdMgr, &ran
}

func requireDenialMessage(t *testing.T, conn *recordingConn, node string) {
	t.Helper()
	written := conn.written()
	require.Len(t, written, 1, "a refused builtin command must be answered with exactly one message")
	pkt, ok := written[0].(*chat.SystemChat)
	require.True(t, ok, "expected a system chat message, got %T", written[0])
	text, ok := pkt.Component.Component.(*component.Text)
	require.True(t, ok, "expected plain text, got %T", pkt.Component.Component)
	require.Contains(t, text.Content, "permission")
	require.Contains(t, text.Content, node)
}

// TestBuiltinCommandPermissionEnforcement is the acceptance test for
// config.permissions (minekube/gate#1293): a player granted a node may run the
// command, and one that is not granted it is refused *and told why* instead of
// having the command forwarded to the backend as an unknown command.
func TestBuiltinCommandPermissionEnforcement(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.BuiltinCommands = true
	cfg.RequireBuiltinCommandPermissions = true
	cfg.Permissions = config.Permissions{
		Default: []string{"gate.command.server"},
		Deny:    map[string][]string{"Bob": {"gate.command.server"}},
	}
	alice := uuid.New()
	bob := uuid.New()

	t.Run("granted player may run /server", func(t *testing.T) {
		permFn := resolvePlayerPermissions(t, &cfg, alice, "Alice")
		player, conn, cmdMgr, ran := newBuiltinPermFixture(t, &cfg, permFn)

		hasRun, err := executeCommand("server", player, cmdMgr)
		require.NoError(t, err)
		require.True(t, hasRun)
		require.Equal(t, []string{"server"}, *ran)
		require.Empty(t, conn.written(), "a permitted command must not answer the player")
	})

	t.Run("player without the node cannot run /send and is told why", func(t *testing.T) {
		permFn := resolvePlayerPermissions(t, &cfg, alice, "Alice")
		player, conn, cmdMgr, ran := newBuiltinPermFixture(t, &cfg, permFn)

		hasRun, err := executeCommand("send Alice", player, cmdMgr)
		require.NoError(t, err)
		require.True(t, hasRun, "a refused builtin command must be consumed, not forwarded to the backend")
		require.Empty(t, *ran)
		requireDenialMessage(t, conn, sendCmdPermission)
	})

	t.Run("deny wins over the default grant", func(t *testing.T) {
		permFn := resolvePlayerPermissions(t, &cfg, bob, "Bob")
		player, conn, cmdMgr, ran := newBuiltinPermFixture(t, &cfg, permFn)

		hasRun, err := executeCommand("server lobby", player, cmdMgr)
		require.NoError(t, err)
		require.True(t, hasRun)
		require.Empty(t, *ran)
		requireDenialMessage(t, conn, serverCmdPermission)
	})

	t.Run("without requireBuiltinCommandPermissions nothing is refused", func(t *testing.T) {
		open := cfg
		open.RequireBuiltinCommandPermissions = false
		permFn := resolvePlayerPermissions(t, &open, alice, "Alice")
		player, conn, cmdMgr, ran := newBuiltinPermFixture(t, &open, permFn)

		hasRun, err := executeCommand("send Alice", player, cmdMgr)
		require.NoError(t, err)
		require.True(t, hasRun)
		require.Equal(t, []string{"send"}, *ran)
		require.Empty(t, conn.written())
	})
}
