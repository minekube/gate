package proto

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/gammazero/deque"
	"github.com/sandertv/go-raknet"
	packetutil "github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/errs"
	"io"
)

type Decoder struct {
	dec       *packetutil.Decoder
	log       logr.Logger
	direction proto.Direction
	// not yet processed raw packets containing packet id + data
	queue deque.Deque

	// TODO move this pkg to common gate/proto
	registry *state.ProtocolRegistry
}

func NewDecoder(
	r io.Reader,
	direction proto.Direction,
	log logr.Logger,
) *Decoder {
	return &Decoder{
		dec:       packetutil.NewDecoder(r),
		log:       log,
		direction: direction,
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
	// Read decrypted and decompressed packets from the underlying decoder.
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

func (d *Decoder) decode(p []byte) (ctx *proto.PacketContext, err error) {
	ctx = &proto.PacketContext{
		Direction:   d.direction,
		Protocol:    d.registry.Protocol,
		KnownPacket: false,
		Payload:     p,
	}
	payload := bytes.NewReader(p)

	// Decode packet header containing the ID
	var header Header
	if err := header.Decode(ctx, payload); err != nil {
		return nil, fmt.Errorf("error reading packet header: %w", err)
	}
	ctx.PacketID = proto.PacketID(header.PacketID)

	// Try find and create packet from the id.
	ctx.Packet = d.registry.CreatePacket(ctx.PacketID)
	if ctx.Packet == nil {
		// Packet id is unknown in this registry,
		// the payload is probably being forwarded as is.
		return
	}

	// Packet is known, decode data into it.
	ctx.KnownPacket = true
	if err = ctx.Packet.Decode(ctx, payload); err != nil {
		if err == io.EOF { // payload was to short or decoder has a bug
			err = io.ErrUnexpectedEOF
		}
		return ctx, errs.NewSilentErr("error decoding packet (type: %T, id: %s, protocol: %s, direction: %s): %w",
			ctx.Packet, ctx.PacketID, ctx.Protocol, ctx.Direction, err)
	}

	// Payload buffer should now be empty.
	if payload.Len() != 0 {
		// packet decoder did not read all of the packet's data!
		d.log.V(1).Info("Packet's decoder did not read all of packet's data",
			"ctx", ctx,
			"decodedBytes", len(ctx.Payload),
			"unreadBytes", payload.Len())
		return ctx, proto.ErrDecoderLeftBytes
	}
	return
}
