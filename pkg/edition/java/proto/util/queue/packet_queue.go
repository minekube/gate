package queue

import (
	"github.com/gammazero/deque"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
)

// PlayPacketQueue is a packet queue for the PLAY state.
// It holds packets that are not registered in the CONFIG state and releases them when ReleaseQueue is called.
// Much of the Gate API (i.e. chat messages) utilize PLAY packets, however the client is
// incapable of receiving these packets during the CONFIG state. Certain events such as the
// ServerPreConnectEvent may be called during this time, and we need to ensure that any API that
// uses these packets will work as expected.
// This handler will queue up any packets that are sent to the client during this time, and send
// them once the client has (re)entered the PLAY state.
type PlayPacketQueue struct {
	registry        *state.ProtocolRegistry
	queue           *deque.Deque[proto.Packet]
	protocolVersion proto.Protocol
}

// NewPlayPacketQueue creates a new PlayPacketQueue with the given protocol version, and direction.
func NewPlayPacketQueue(version proto.Protocol, direction proto.Direction) *PlayPacketQueue {
	return &PlayPacketQueue{
		registry:        state.FromDirection(direction, state.Config, version),
		queue:           deque.New[proto.Packet](),
		protocolVersion: version,
	}
}

// Queue returns true if the packet was queued.
// If the packet is not registered in the CONFIG state, it will be queued.
// Otherwise, it will not be queued and false will be returned.
func (h *PlayPacketQueue) Queue(packet proto.Packet) bool {
	if h == nil {
		return false
	}
	if _, ok := h.registry.PacketID(packet); !ok {
		h.queue.PushBack(packet)
		return true
	}
	return false
}

// PacketBuffer is a packet buffer that can flush packets to an underlying packet writer.
type PacketBuffer interface {
	BufferPacket(proto.Packet) error
	Flush() error
}

// ReleaseQueue releases all packets in the queue to the sink packet writer.
// It iterates over the queue, buffering each packet and flushing the sink.
func (h *PlayPacketQueue) ReleaseQueue(sink PacketBuffer) error {
	if h == nil {
		return nil
	}
	var ok bool
	for h.queue.Len() > 0 {
		packet := h.queue.PopFront()
		if err := sink.BufferPacket(packet); err != nil {
			return err
		}
		ok = true
	}
	if ok {
		return sink.Flush()
	}
	return nil
}
