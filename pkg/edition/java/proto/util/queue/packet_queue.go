package queue

import (
	"errors"

	"github.com/edwingeng/deque/v2"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
)

// maxQueueLen is the maximum number of packets that can be queued
// to prevent unbounded memory growth from a malicious or buggy peer.
const maxQueueLen = 1024

// ErrQueueFull is returned when the play packet queue exceeds its maximum size.
var ErrQueueFull = errors.New("play packet queue full")

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
		queue:           deque.NewDeque[proto.Packet](),
		protocolVersion: version,
	}
}

// Queue returns true if the packet was queued.
// If the packet is not registered in the CONFIG state, it will be queued.
// Otherwise, it will not be queued and false will be returned.
// Returns an error if the queue is full.
func (h *PlayPacketQueue) Queue(packet proto.Packet) (bool, error) {
	if h == nil {
		return false, nil
	}
	if _, ok := h.registry.PacketID(packet); !ok {
		if h.queue.Len() >= maxQueueLen {
			return false, ErrQueueFull
		}
		h.queue.PushBack(packet)
		return true, nil
	}
	return false, nil
}

// ReleaseQueue releases all packets in the queue to the sink packet writer.
// It iterates over the queue, buffering each packet and flushing the sink.
func (h *PlayPacketQueue) ReleaseQueue(
	buffer func(proto.Packet) error,
	flush func() error,
) error {
	if h == nil {
		return nil
	}
	var ok bool
	for h.queue.Len() != 0 {
		packet := h.queue.PopFront()
		if err := buffer(packet); err != nil {
			return err
		}
		ok = true
	}
	if ok {
		return flush()
	}
	return nil
}
