package proxy

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

type staticBackendHandshakeAddresser struct {
	gotDefaultHost string
	gotPlayer      Player
	gotTarget      RegisteredServer
	nextHost       string
	err            error
}

func (a *staticBackendHandshakeAddresser) BackendHandshakeAddr(defaultServerAddress string, player Player, target RegisteredServer) (string, error) {
	a.gotDefaultHost = defaultServerAddress
	a.gotPlayer = player
	a.gotTarget = target
	if a.err != nil {
		return "", a.err
	}
	return a.nextHost, nil
}

func TestHandshakeAddrUsesBackendHandshakeAddresserWithTarget(t *testing.T) {
	serverConn, player, target := newHandshakeAddrTestConnection(t, config.NoneForwardingMode, phase.Vanilla)
	addresser := &staticBackendHandshakeAddresser{nextHost: "play.example.org\x00encrypted-floodgate-data"}
	player.proxy.SetBackendHandshakeAddresser(addresser)

	got, err := serverConn.handshakeAddr("play.example.org", player)

	require.NoError(t, err)
	require.Equal(t, "play.example.org", addresser.gotDefaultHost)
	require.Same(t, player, addresser.gotPlayer)
	require.Same(t, target, addresser.gotTarget)
	require.Equal(t, "play.example.org\x00encrypted-floodgate-data", got)
}

func TestStartHandshakeReturnsBackendHandshakeAddresserErrorBeforeBufferingHandshake(t *testing.T) {
	serverConn, player, _ := newHandshakeAddrTestConnection(t, config.NoneForwardingMode, phase.Vanilla)
	wantErr := errors.New("backend floodgate encode failed")
	player.proxy.SetBackendHandshakeAddresser(&staticBackendHandshakeAddresser{err: wantErr})
	backendConn := &testMinecraftConn{}
	serverConn.connection = backendConn

	_, err := serverConn.startHandshake(func() {}, make(chan *connResponse, 1))

	require.ErrorIs(t, err, wantErr)
	require.Empty(t, backendConn.writtenPackets)
}

func TestHandshakeAddrPreservesLegacyForwardingWhenBackendAddresserDisabled(t *testing.T) {
	serverConn, player, _ := newHandshakeAddrTestConnection(t, config.LegacyForwardingMode, phase.ModernForge)
	player.virtualHost = netutil.NewAddr("play.example.org\x00FML3\x00:25565", "tcp")

	got, err := serverConn.handshakeAddr("play.example.org", serverConn.player)

	require.NoError(t, err)
	require.Equal(t, serverConn.createLegacyForwardingAddress(), got)
	require.Contains(t, got, `"extraData"`)
	require.False(t, strings.HasSuffix(got, "\x00FML3\x00"), "legacy forwarding already carries Forge metadata")
}

func TestStartHandshakeBuffersBackendHandshakeAddress(t *testing.T) {
	serverConn, player, _ := newHandshakeAddrTestConnection(t, config.NoneForwardingMode, phase.Vanilla)
	player.proxy.SetBackendHandshakeAddresser(&staticBackendHandshakeAddresser{nextHost: "play.example.org\x00encrypted-floodgate-data"})
	backendConn := &testMinecraftConn{}
	serverConn.connection = backendConn
	resultChan := make(chan *connResponse, 1)
	resultChan <- &connResponse{connectionResult: &connectionResult{}}

	_, err := serverConn.startHandshake(func() {}, resultChan)

	require.NoError(t, err)
	require.Len(t, backendConn.writtenPackets, 2)
	handshake, ok := backendConn.writtenPackets[0].(*packet.Handshake)
	require.True(t, ok, "first packet = %T", backendConn.writtenPackets[0])
	require.Equal(t, "play.example.org\x00encrypted-floodgate-data", handshake.ServerAddress)
}

func newHandshakeAddrTestConnection(t *testing.T, forwardingMode config.ForwardingMode, connType phase.ConnectionType) (*serverConnection, *connectedPlayer, RegisteredServer) {
	t.Helper()

	cfg := &config.Config{
		Forwarding: config.Forwarding{Mode: forwardingMode},
	}
	p := &Proxy{cfg: cfg}
	player := &connectedPlayer{
		MinecraftConn:      &testMinecraftConn{connType: connType},
		sessionHandlerDeps: &sessionHandlerDeps{proxy: p, configProvider: p},
		log:                logr.Discard(),
		profile: &profile.GameProfile{
			ID: uuid.New(),
		},
		virtualHost: netutil.NewAddr("play.example.org:25565", "tcp"),
	}
	target := newRegisteredServer(NewServerInfo("backend", netutil.NewAddr("127.0.0.1:25566", "tcp")))
	serverConn := &serverConnection{
		server: target,
		player: player,
		log:    logr.Discard(),
	}
	return serverConn, player, target
}
