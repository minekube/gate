package bungeecord

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"strconv"
	"testing"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/netutil"
)

// The BungeeCord "ServerIP" response carries the backend server's port as an
// unsigned 16-bit big-endian value (BungeeCord writes it with
// `out.writeShort(port)`), which is what `Value` is here. These tests pin that
// wire format and pin that Gate writes those exact bytes - the response's port
// field must never depend on how the writer's *argument* is signed, because a
// port above math.MaxInt16 is a perfectly ordinary port as far as the wire is
// concerned.

// serverIPResponderHarness is a minimal Providers implementation that lets
// processServerIP run end to end and captures the response bytes it writes.
type serverIPResponderHarness struct {
	server *serverIPFakeServer
	conn   *serverIPFakeConn
}

func (h *serverIPResponderHarness) PlayerByName(string) Player           { return nil }
func (h *serverIPResponderHarness) PlayerCount() int                     { return 0 }
func (h *serverIPResponderHarness) Players() []Player                    { return nil }
func (h *serverIPResponderHarness) BroadcastMessage(component.Component) {}
func (h *serverIPResponderHarness) Servers() []Server                    { return []Server{h.server} }
func (h *serverIPResponderHarness) ConnectedServer() ServerConnection    { return h.conn }
func (h *serverIPResponderHarness) Server(name string) Server {
	if h.server != nil && h.server.Name() == name {
		return h.server
	}
	return nil
}

type serverIPFakeServer struct {
	name string
	addr net.Addr
}

func (s *serverIPFakeServer) Name() string                                             { return s.name }
func (s *serverIPFakeServer) PlayerCount() int                                         { return 0 }
func (s *serverIPFakeServer) BroadcastPluginMessage(message.ChannelIdentifier, []byte) {}
func (s *serverIPFakeServer) Connect(Player)                                           {}
func (s *serverIPFakeServer) Players() []Player                                        { return nil }
func (s *serverIPFakeServer) BroadcastMessage(component.Component)                     {}
func (s *serverIPFakeServer) Addr() net.Addr                                           { return s.addr }

type serverIPFakeConn struct {
	written []*plugin.Message
}

func (c *serverIPFakeConn) Name() string             { return "lobby" }
func (c *serverIPFakeConn) Protocol() proto.Protocol { return version.Minecraft_1_21.Protocol }
func (c *serverIPFakeConn) WritePacket(p proto.Packet) error {
	msg, ok := p.(*plugin.Message)
	if !ok {
		return fmt.Errorf("unexpected packet type %T", p)
	}
	c.written = append(c.written, msg)
	return nil
}

// serverIPResponse drives processServerIP for the given server address and
// returns the plugin message payload written to the backend connection.
func serverIPResponse(t *testing.T, serverName, serverAddr string) []byte {
	t.Helper()
	conn := &serverIPFakeConn{}
	h := &serverIPResponderHarness{
		server: &serverIPFakeServer{name: serverName, addr: netutil.NewAddr(serverAddr, "tcp")},
		conn:   conn,
	}
	responder := NewMessageResponder(nil, h)

	in := new(bytes.Buffer)
	if err := util.WriteUTF(in, "ServerIP"); err != nil {
		t.Fatalf("write sub-channel: %v", err)
	}
	if err := util.WriteUTF(in, serverName); err != nil {
		t.Fatalf("write server name: %v", err)
	}
	if ok := responder.Process(&plugin.Message{Channel: Channel(version.Minecraft_1_21.Protocol), Data: in.Bytes()}); !ok {
		t.Fatal("ServerIP message was not processed as a BungeeCord message")
	}
	if len(conn.written) != 1 {
		t.Fatalf("expected exactly 1 response message, got %d", len(conn.written))
	}
	return conn.written[0].Data
}

// expectedServerIPPayload builds the ServerIP payload independently of the
// production code: sub-channel, server name, host, then the port as an unsigned
// 16-bit big-endian field - the layout BungeeCord's own DownstreamBridge
// writes (name, host, writeShort(port)).
func expectedServerIPPayload(t *testing.T, serverName, host string, port uint16) []byte {
	t.Helper()
	want := new(bytes.Buffer)
	for _, s := range []string{"ServerIP", serverName, host} {
		if err := util.WriteUTF(want, s); err != nil {
			t.Fatalf("write %q: %v", s, err)
		}
	}
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], port)
	want.Write(portBytes[:])
	return want.Bytes()
}

func TestServerIPResponseWritesPortAsUnsignedBigEndian16(t *testing.T) {
	const (
		serverName = "lobby"
		host       = "10.9.8.7"
	)
	// Ports on both sides of math.MaxInt16 (32767) plus the extremes. A port
	// above 32767 is the case where a signed 16-bit argument could differ from
	// the unsigned wire value if the writer mishandled it - the case that made
	// CodeQL flag the narrowing in the first place.
	ports := []uint16{0, 1, 25565, 32767, 32768, 40000, 65534, math.MaxUint16}

	for _, port := range ports {
		t.Run(strconv.Itoa(int(port)), func(t *testing.T) {
			addr := net.JoinHostPort(host, strconv.Itoa(int(port)))
			got := serverIPResponse(t, serverName, addr)
			want := expectedServerIPPayload(t, serverName, host, port)
			if !bytes.Equal(got, want) {
				t.Fatalf("ServerIP payload mismatch for port %d:\n got %x\nwant %x", port, got, want)
			}
			// The last two bytes must be the unsigned big-endian encoding of the
			// port, byte for byte.
			if len(got) < 2 {
				t.Fatalf("payload too short to carry a port: %x", got)
			}
			portField := got[len(got)-2:]
			if uint16(portField[0])<<8|uint16(portField[1]) != port {
				t.Fatalf("port field %x does not decode to port %d", portField, port)
			}
		})
	}
}

// TestWriteInt16AndWriteUint16EmitIdenticalBytesForEveryPort pins the claim that
// writing the port through the signed writer with int16(port) and through the
// unsigned writer with port produce identical bytes - for every one of the
// 65536 possible port values, not a sample. uint16 -> int16 -> uint16 is the
// identity on the bit pattern and both writers bottom out in
// binary.BigEndian.PutUint16, so the ServerIP response's emitted bytes cannot
// change when the call site switches from WriteInt16(b, int16(port)) to
// WriteUint16(b, port).
//
// This holds on the pre-change code as well as the post-change code: the
// refactor is byte-preserving by construction, so there is no RED to
// manufacture here (see the PR body). The test is the guard - it fails if the
// signed writer ever stops being bit-identical to the unsigned one, which is
// exactly the assumption that lets the call site be simplified.
func TestWriteInt16AndWriteUint16EmitIdenticalBytesForEveryPort(t *testing.T) {
	for i := 0; i <= math.MaxUint16; i++ {
		port := uint16(i)

		signed := new(bytes.Buffer)
		if err := util.WriteInt16(signed, int16(port)); err != nil {
			t.Fatalf("WriteInt16(%d): %v", port, err)
		}

		unsigned := new(bytes.Buffer)
		if err := util.WriteUint16(unsigned, port); err != nil {
			t.Fatalf("WriteUint16(%d): %v", port, err)
		}

		if !bytes.Equal(signed.Bytes(), unsigned.Bytes()) {
			t.Fatalf("port %d: WriteInt16(int16(port)) = %x but WriteUint16(port) = %x",
				port, signed.Bytes(), unsigned.Bytes())
		}
		if len(unsigned.Bytes()) != 2 {
			t.Fatalf("port %d: expected a 2-byte field, got %x", port, unsigned.Bytes())
		}
	}
}
