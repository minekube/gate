package proxy

import (
	"testing"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
)

func newTestPlayHandler() *clientPlaySessionHandler {
	player := &connectedPlayer{
		MinecraftConn: &testMinecraftConn{},
		log:           logr.Discard(),
	}
	return &clientPlaySessionHandler{player: player, log: logr.Discard()}
}

// A client that never finishes its login/FML phase can spam pre-join plugin
// messages. The queue must be bounded by message count and the player kicked
// once the cap is exceeded.
func TestEnqueueLoginPluginMessageCountCap(t *testing.T) {
	h := newTestPlayHandler()
	small := func() *plugin.Message { return &plugin.Message{Channel: "x", Data: []byte{0x1}} }

	for i := 0; i < maxQueuedLoginPluginMessages; i++ {
		if !h.enqueueLoginPluginMessage(small()) {
			t.Fatalf("enqueue %d rejected unexpectedly", i)
		}
	}
	if h.enqueueLoginPluginMessage(small()) {
		t.Fatal("expected rejection once message-count cap is exceeded")
	}
	if got := h.mu.loginPluginMessages.Len(); got > maxQueuedLoginPluginMessages {
		t.Fatalf("queue grew past cap: len=%d", got)
	}
	// Overflow is latched: further enqueues keep failing.
	if h.enqueueLoginPluginMessage(small()) {
		t.Fatal("expected rejection after overflow latched")
	}
}

// A single oversized batch (by total bytes) must also trip the cap.
func TestEnqueueLoginPluginMessageByteCap(t *testing.T) {
	h := newTestPlayHandler()
	big := &plugin.Message{Channel: "x", Data: make([]byte, maxQueuedLoginPluginMessageBytes+1)}
	if h.enqueueLoginPluginMessage(big) {
		t.Fatal("expected rejection when message exceeds the byte cap")
	}
}

// Draining the queue (on flush) must reset the byte counter, otherwise the byte
// cap would trip prematurely for messages queued after a flush and wrongly
// disconnect a legitimate player.
func TestDrainQueuedLoginPluginMessagesResetsByteCounter(t *testing.T) {
	h := newTestPlayHandler()
	for i := 0; i < 3; i++ {
		if !h.enqueueLoginPluginMessage(&plugin.Message{Data: make([]byte, 1000)}) {
			t.Fatalf("enqueue %d rejected unexpectedly", i)
		}
	}

	drained := h.drainQueuedLoginPluginMessages()
	if len(drained) != 3 {
		t.Fatalf("drained %d messages, want 3", len(drained))
	}

	h.mu.RLock()
	gotBytes, gotLen := h.mu.loginPluginMessagesBytes, h.mu.loginPluginMessages.Len()
	h.mu.RUnlock()
	if gotBytes != 0 || gotLen != 0 {
		t.Fatalf("after drain: bytes=%d len=%d, want 0/0", gotBytes, gotLen)
	}

	// A fresh message just under the byte cap must still be accepted.
	if !h.enqueueLoginPluginMessage(&plugin.Message{Data: make([]byte, maxQueuedLoginPluginMessageBytes-1)}) {
		t.Fatal("byte cap tripped early after drain (counter leaked)")
	}
}
