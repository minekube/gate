package proxy

import (
	"errors"
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/gate/proto"
)

// TestClientConfigKnownPacksReplySurvivesBackendNotReady is the RED test for the
// silent post-registration CONFIGURATION stall named in kanban t_b8e78d77.
//
// Observed failure (lab stack, 8-way cold-translation arrival): the backend
// registers, the translation runtime enters CONFIGURATION, the backend sends
// ClientboundSelectKnownPacks and then sends nothing else - ever. The client
// received 0 further bytes until its own budget closed the socket: no error, no
// kick, no disconnect, no timeout (read/write deadlines are double-scaled to
// ~690 days, see card t_7f800139).
//
// Mechanism: the client's ServerboundKnownPacks reply is the packet that
// releases the backend's registry data. handleKnownPacks() resolves the backend
// connection and, when it is not ready yet, returns without writing and without
// remembering the packet - unlike config-state plugin messages, which are
// queued by enqueuePluginMessage() and replayed by flushQueuedPluginMessagesTo()
// when the backend is ready. The reply is therefore lost with no retry.
func TestClientConfigKnownPacksReplySurvivesBackendNotReady(t *testing.T) {
	handler, player, serverConn, backendConn := newTestConfigHandler(t)

	reply := &config.KnownPacks{Packs: []config.KnownPack{
		{Namespace: "minecraft", Id: "core", Version: "1.21.4"},
	}}

	// The client answers ClientboundSelectKnownPacks in the window where the
	// backend config connection is not attached yet (cold per-join translation
	// runtime: registration takes ~1.5 s, the client replies in milliseconds).
	handler.HandlePacket(&proto.PacketContext{Packet: reply})

	if got := len(backendConn.writtenPackets); got != 0 {
		t.Fatalf("reply written to a not-yet-attached backend: got %d packets, want 0", got)
	}

	// Backend config connection becomes ready - the exact hook that already
	// replays queued config plugin messages.
	player.mu.Lock()
	player.connInFlight = serverConn
	player.mu.Unlock()
	if err := handler.flushQueuedPluginMessagesTo(serverConn); err != nil {
		t.Fatalf("flush queued config packets: %v", err)
	}

	if got := len(backendConn.writtenPackets); got != 1 {
		t.Fatalf("config-state KnownPacks reply was dropped: backend received %d packets, want 1\n"+
			"gate never writes it later and the backend waits for it forever, so the join hangs "+
			"silently in CONFIGURATION until the client gives up", got)
	}
	got, ok := backendConn.writtenPackets[0].(*config.KnownPacks)
	if !ok {
		t.Fatalf("replayed packet type = %T, want *config.KnownPacks", backendConn.writtenPackets[0])
	}
	if len(got.Packs) != 1 || got.Packs[0].Id != "core" {
		t.Fatalf("replayed KnownPacks = %+v, want the client's reply", got.Packs)
	}
}

// TestClientConfigKnownPacksReplyQueuedWhenBackendNotWritable covers the other
// drop branch: the backend config connection exists but rejects the write. The
// reply must be kept and replayed exactly once when the connection is ready,
// and a later flush must not write it again.
func TestClientConfigKnownPacksReplyQueuedWhenBackendNotWritable(t *testing.T) {
	handler, player, serverConn, baseConn := newTestConfigHandler(t)
	backendConn := &notWritableTestConn{testMinecraftConn: baseConn, err: errTestConnNotWritable}
	serverConn.mu.Lock()
	serverConn.connection = backendConn
	serverConn.mu.Unlock()

	player.mu.Lock()
	player.connInFlight = serverConn
	player.mu.Unlock()

	handler.HandlePacket(&proto.PacketContext{Packet: &config.KnownPacks{Packs: []config.KnownPack{
		{Namespace: "minecraft", Id: "core", Version: "1.21.4"},
	}}})

	if got := len(baseConn.writtenPackets); got != 0 {
		t.Fatalf("reply recorded while the backend rejected the write: got %d packets, want 0", got)
	}

	backendConn.err = nil
	if err := handler.flushQueuedPluginMessagesTo(serverConn); err != nil {
		t.Fatalf("flush queued config packets: %v", err)
	}
	if got := len(baseConn.writtenPackets); got != 1 {
		t.Fatalf("queued KnownPacks reply replay = %d packets, want 1", got)
	}
	if _, ok := baseConn.writtenPackets[0].(*config.KnownPacks); !ok {
		t.Fatalf("replayed packet type = %T, want *config.KnownPacks", baseConn.writtenPackets[0])
	}

	if err := handler.flushQueuedPluginMessagesTo(serverConn); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	if got := len(baseConn.writtenPackets); got != 1 {
		t.Fatalf("KnownPacks reply replayed twice: %d packets, want 1", got)
	}
}

var errTestConnNotWritable = errors.New("test backend not writable")

// notWritableTestConn is a backend connection that refuses direct writes until
// err is cleared, while still buffering packets - the state of a backend config
// connection that exists but cannot take a packet yet.
type notWritableTestConn struct {
	*testMinecraftConn
	err error
}

func (c *notWritableTestConn) WritePacket(packet proto.Packet) error {
	if c.err != nil {
		return c.err
	}
	return c.testMinecraftConn.WritePacket(packet)
}
