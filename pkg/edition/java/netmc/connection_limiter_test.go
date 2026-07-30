package netmc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/packetlimiter"
)

type noopSessionHandler struct{}

func (noopSessionHandler) HandlePacket(*proto.PacketContext) {}
func (noopSessionHandler) Disconnected()                     {}
func (noopSessionHandler) Activated()                        {}
func (noopSessionHandler) Deactivated()                      {}

// A client that floods serverbound packets past its rate limit must have its
// connection closed by the read loop.
func TestReadLoopClosesConnectionOnPacketFlood(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	// 1 packet/s over a 1s window: the 2nd packet in the window trips the limit.
	limiter := packetlimiter.New(1, -1, time.Second)
	conn, startReadLoop := NewMinecraftConn(
		context.Background(), server, proto.ServerBound,
		5*time.Second, 5*time.Second, 0, limiter,
	)
	conn.SetActiveSessionHandler(state.Handshake, noopSessionHandler{})

	done := make(chan struct{})
	go func() { startReadLoop(); close(done) }()

	// Flood handshake packets from the client side.
	go func() {
		enc := codec.NewEncoder(client, proto.ServerBound, logr.Discard())
		hs := &packet.Handshake{
			ProtocolVersion: int(version.Minecraft_1_21.Protocol),
			ServerAddress:   "localhost",
			Port:            25565,
			NextStatus:      1,
		}
		for i := 0; i < 10; i++ {
			if _, err := enc.WritePacket(hs); err != nil {
				return // pipe closed once the limiter kicked in
			}
		}
	}()

	select {
	case <-done:
		if !Closed(conn) {
			t.Fatal("read loop returned but connection is not closed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("connection was not closed after packet flood")
	}
}

// Without a limiter, the same packet stream must NOT close the connection — this
// guards against the flood test passing for an unrelated reason (e.g. a framing
// or decode error).
func TestReadLoopKeepsConnectionWithoutLimiter(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	conn, startReadLoop := NewMinecraftConn(
		context.Background(), server, proto.ServerBound,
		5*time.Second, 5*time.Second, 0, nil, // no limiter
	)
	conn.SetActiveSessionHandler(state.Handshake, noopSessionHandler{})

	done := make(chan struct{})
	go func() { startReadLoop(); close(done) }()

	enc := codec.NewEncoder(client, proto.ServerBound, logr.Discard())
	hs := &packet.Handshake{
		ProtocolVersion: int(version.Minecraft_1_21.Protocol),
		ServerAddress:   "localhost",
		Port:            25565,
		NextStatus:      1,
	}
	for i := 0; i < 10; i++ {
		if _, err := enc.WritePacket(hs); err != nil {
			t.Fatalf("write %d failed (connection closed unexpectedly): %v", i, err)
		}
	}

	select {
	case <-done:
		t.Fatal("connection closed without a limiter configured")
	case <-time.After(200 * time.Millisecond):
		// Still open after processing the flood, as expected.
	}
}
