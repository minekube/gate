package proxy

import (
	"context"
	"net"
	"testing"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/bungeecord"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/gate/proto"
)

func TestBackendPlayRegisterForwardsToPlayer(t *testing.T) {
	playerConn := &testMinecraftConn{}
	player := &connectedPlayer{
		MinecraftConn: playerConn,
		log:           logr.Discard(),
	}
	serverConn := &serverConnection{
		player: player,
		log:    logr.Discard(),
	}
	handler := &backendPlaySessionHandler{
		serverConn:                 serverConn,
		bungeeCordMessageResponder: bungeecord.NopMessageResponder,
		log:                        logr.Discard(),
	}

	register := &plugin.Message{
		Channel: plugin.RegisterChannel,
		Data:    []byte("axiom:hello"),
	}
	handler.handlePluginMessage(register, nil)

	if len(playerConn.writtenPackets) != 1 {
		t.Fatalf("expected register packet to be written to player once, got %d", len(playerConn.writtenPackets))
	}
	got, ok := playerConn.writtenPackets[0].(*plugin.Message)
	if !ok {
		t.Fatalf("expected plugin message, got %T", playerConn.writtenPackets[0])
	}
	if got.Channel != register.Channel {
		t.Fatalf("expected channel %q, got %q", register.Channel, got.Channel)
	}
	if string(got.Data) != string(register.Data) {
		t.Fatalf("expected payload %q, got %q", string(register.Data), string(got.Data))
	}
}

type testMinecraftConn struct {
	writtenPackets []proto.Packet
	connType       phase.ConnectionType
}

func (t *testMinecraftConn) Context() context.Context { return context.Background() }
func (t *testMinecraftConn) Close() error             { return nil }
func (t *testMinecraftConn) State() *state.Registry   { return state.Play }
func (t *testMinecraftConn) Protocol() proto.Protocol { return version.Minecraft_1_20_3.Protocol }
func (t *testMinecraftConn) RemoteAddr() net.Addr     { return &net.TCPAddr{} }
func (t *testMinecraftConn) LocalAddr() net.Addr      { return &net.TCPAddr{} }
func (t *testMinecraftConn) Type() phase.ConnectionType {
	if t.connType != nil {
		return t.connType
	}
	return phase.Vanilla
}
func (t *testMinecraftConn) SetType(ct phase.ConnectionType) { t.connType = ct }
func (t *testMinecraftConn) ActiveSessionHandler() netmc.SessionHandler { return nil }
func (t *testMinecraftConn) SetActiveSessionHandler(*state.Registry, netmc.SessionHandler) {
}
func (t *testMinecraftConn) SwitchSessionHandler(*state.Registry) bool { return true }
func (t *testMinecraftConn) AddSessionHandler(*state.Registry, netmc.SessionHandler) {
}
func (t *testMinecraftConn) SetAutoReading(bool)               {}
func (t *testMinecraftConn) SetProtocol(proto.Protocol)        {}
func (t *testMinecraftConn) SetState(*state.Registry)          {}
func (t *testMinecraftConn) SetCompressionThreshold(int) error { return nil }
func (t *testMinecraftConn) EnableEncryption([]byte) error     { return nil }
func (t *testMinecraftConn) WritePacket(packet proto.Packet) error {
	t.writtenPackets = append(t.writtenPackets, packet)
	return nil
}
func (t *testMinecraftConn) Write([]byte) error { return nil }
func (t *testMinecraftConn) BufferPacket(packet proto.Packet) error {
	t.writtenPackets = append(t.writtenPackets, packet)
	return nil
}
func (t *testMinecraftConn) BufferPayload([]byte) error { return nil }
func (t *testMinecraftConn) Flush() error               { return nil }
func (t *testMinecraftConn) Reader() netmc.Reader       { return nil }
func (t *testMinecraftConn) Writer() netmc.Writer       { return nil }
func (t *testMinecraftConn) EnablePlayPacketQueue()     {}

var _ netmc.MinecraftConn = (*testMinecraftConn)(nil)
