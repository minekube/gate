package proxy

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/netutil"
)

// TestUnsupportedProtocolIsRefusedWithDecodableDisconnect pins the fail-closed
// behaviour for a client whose protocol Gate does not know: the proxy must refuse
// it with a "multiplayer.disconnect.outdated_client" reason the client can
// actually decode, instead of starting a login it cannot encode.
//
// Regression (2026-09-18, Minecraft 26.3 / protocol 777 and every unknown
// protocol): Supported() accepted any numeric protocol, so the login proceeded
// and the login hello was encoded through the minimum-version fallback registry
// (pre-1.8 layout) - the client kicked itself with a decode error instead of
// being told its version is outdated.
func TestUnsupportedProtocolIsRefusedWithDecodableDisconnect(t *testing.T) {
	tests := []struct {
		name     string
		protocol int
	}{
		{"one above the table (778)", 778},
		{"far above the table (900)", 900},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if version.Protocol(tt.protocol).Supported() {
				t.Fatalf("protocol %d is reported as supported: the proxy would negotiate a login it cannot encode", tt.protocol)
			}

			conn := newRefusalTestConn(proto.Protocol(tt.protocol))
			handler := newRefusalTestHandler(conn)
			inbound := newInitialInbound(conn, netutil.NewAddr("minekube.net:25565", "tcp"), packet.LoginHandshakeIntent)

			handler.handleLogin(&packet.Handshake{
				ServerAddress:   "minekube.net",
				Port:            25565,
				ProtocolVersion: tt.protocol,
			}, inbound)

			if len(conn.writes) != 1 {
				t.Fatalf("wrote %d packets to an unsupported client, want exactly 1 (the refusal)", len(conn.writes))
			}
			disconnect, ok := conn.writes[0].(*packet.Disconnect)
			if !ok {
				t.Fatalf("refusal packet = %T, want *packet.Disconnect", conn.writes[0])
			}
			if !conn.closed {
				t.Fatal("the connection must be closed after the refusal")
			}
			// The refusal must not start a login negotiation. The connection may be
			// moved into the login *state* (that is what makes the disconnect
			// writable), but no login session handler may be activated.
			for _, activation := range conn.handlers {
				if _, ok := activation.handler.(*initialLoginSessionHandler); ok {
					t.Fatalf("an unsupported protocol must not start a login session (registry %v)", activation.registry)
				}
			}

			// The refusal has to survive the wire: encode it the way the login state
			// does and read it back the way the client does.
			reason := encodeRefusal(t, tt.protocol, disconnect)
			if !strings.Contains(reason, "multiplayer.disconnect.outdated_client") {
				t.Fatalf("refusal reason = %q, want the multiplayer.disconnect.outdated_client translation", reason)
			}
		})
	}
}

// encodeRefusal writes the refusal the way the login state encodes it and returns
// the decoded component JSON.
func encodeRefusal(t *testing.T, protocol int, disconnect *packet.Disconnect) string {
	t.Helper()

	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf, proto.ClientBound, logr.Discard())
	enc.SetState(state.Login)
	enc.SetProtocol(proto.Protocol(protocol))
	if _, err := enc.WritePacket(disconnect); err != nil {
		t.Fatalf("encode refusal for protocol %d: %v", protocol, err)
	}

	body := buf.Bytes()
	length, n := readRefusalVarInt(t, body)
	if length != len(body)-n {
		t.Fatalf("refusal frame declares %d bytes, %d available", length, len(body)-n)
	}
	id, n2 := readRefusalVarInt(t, body[n:])
	if id != 0x00 {
		t.Fatalf("refusal packet id = %#x, want 0x00 (login disconnect)", id)
	}
	reasonLen, n3 := readRefusalVarInt(t, body[n+n2:])
	reason := body[n+n2+n3:]
	if reasonLen != len(reason) {
		t.Fatalf("refusal reason declares %d bytes, %d available", reasonLen, len(reason))
	}
	return string(reason)
}

func readRefusalVarInt(t *testing.T, b []byte) (int, int) {
	t.Helper()
	var (
		value int
		shift uint
	)
	for i, by := range b {
		value |= int(by&0x7F) << shift
		if by&0x80 == 0 {
			return value, i + 1
		}
		shift += 7
		if shift > 35 {
			t.Fatalf("varint too long: %v", b)
		}
	}
	t.Fatalf("truncated varint: %v", b)
	return 0, 0
}

type refusalTestConn struct {
	netmc.MinecraftConn

	ctx      context.Context
	protocol proto.Protocol
	state    *state.Registry
	writes   []proto.Packet
	handlers []handlerActivation
	closed   bool
}

type handlerActivation struct {
	registry *state.Registry
	handler  netmc.SessionHandler
}

func newRefusalTestConn(protocol proto.Protocol) *refusalTestConn {
	return &refusalTestConn{
		ctx:      context.Background(),
		protocol: protocol,
		state:    state.Handshake,
	}
}

func (c *refusalTestConn) Context() context.Context     { return c.ctx }
func (c *refusalTestConn) Close() error                 { c.closed = true; return nil }
func (c *refusalTestConn) State() *state.Registry       { return c.state }
func (c *refusalTestConn) Protocol() proto.Protocol     { return c.protocol }
func (c *refusalTestConn) RemoteAddr() net.Addr         { return netutil.NewAddr("203.0.113.7:50000", "tcp") }
func (c *refusalTestConn) LocalAddr() net.Addr          { return netutil.NewAddr("10.0.0.1:25565", "tcp") }
func (c *refusalTestConn) Type() phase.ConnectionType   { return phase.Vanilla }
func (c *refusalTestConn) SetType(phase.ConnectionType) {}
func (c *refusalTestConn) ActiveSessionHandler() netmc.SessionHandler {
	return nil
}
func (c *refusalTestConn) SetActiveSessionHandler(registry *state.Registry, handler netmc.SessionHandler) {
	c.state = registry
	c.handlers = append(c.handlers, handlerActivation{registry: registry, handler: handler})
}
func (c *refusalTestConn) WritePacket(p proto.Packet) error {
	c.writes = append(c.writes, p)
	return nil
}

func newRefusalTestHandler(conn *refusalTestConn) *handshakeSessionHandler {
	eventMgr := event.New()
	event.Subscribe(eventMgr, 0, func(*ConnectionHandshakeEvent) {})
	return &handshakeSessionHandler{
		sessionHandlerDeps: &sessionHandlerDeps{
			configProvider: &testConfigProvider{cfg: &config.Config{}},
			eventMgr:       eventMgr,
		},
		conn: conn,
		log:  logr.Discard(),
	}
}
