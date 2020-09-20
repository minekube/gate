package codec

import (
	"errors"
	"github.com/gammazero/deque"
	"github.com/sandertv/go-raknet"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/runtime/logr"
	"io"
)

type Decoder struct {
	dec       *packet.Decoder
	log       logr.Logger
	direction proto.Direction

	queue *deque.Deque // raw packets not yet decoded
}

func NewDecoder(r io.Reader, direction proto.Direction, log logr.Logger) *Decoder {
	return &Decoder{
		dec:       packet.NewDecoder(r),
		log:       log,
		direction: direction,
		queue:     &deque.Deque{},
	}
}

func (d *Decoder) Decode() (*proto.PacketContext, error) {
	for d.queue.Len() == 0 {
		if err := d.fillQueue(); err != nil {
			return nil, err
		}
	}
	rawPacket := d.queue.PopBack().([]byte)
	return d.decode(rawPacket)
}

func (d *Decoder) fillQueue() error {
	// Load the queue.
	// Read raw un-decrypted, un-deflated packets from the underlying reader.
	rawPackets, err := d.dec.Decode()
	if err != nil {
		if !raknet.ErrConnectionClosed(err) {
			return io.EOF
		}
	}

	// Error out if we get too many packets and the server can't keep up.
	if len(rawPackets)+d.queue.Len() > 1000 {
		d.queue.Clear()
		return errors.New("1000+ unhandled packets in queue, Decode caller is to slow")
	}

	for _, rawPacket := range rawPackets {
		d.queue.PushFront(rawPacket)
	}
	return nil
}

func (d *Decoder) decode(rawPacket []byte) (*proto.PacketContext, error) {
	// TODO
	c := &proto.PacketContext{
		Direction:   d.direction,
		Protocol:    0,
		PacketID:    0,
		KnownPacket: false,
		Packet:      nil,
		Payload:     nil,
	}
}
