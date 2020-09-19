package codec

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/errs"
	"io"
	"sync"
)

// Decoder is a synchronized packet decoder.
type Decoder struct {
	log       logr.Logger
	direction proto.Direction

	mu                   sync.Mutex // Protects following field and locked while reading a packet.
	rd                   io.Reader  // The underlying reader.
	registry             *state.ProtocolRegistry
	state                *state.Registry
	compression          bool
	compressionThreshold int
}

func NewDecoder(
	r io.Reader,
	direction proto.Direction,
	log logr.Logger,
) *Decoder {
	return &Decoder{
		rd:        &fullReader{r}, // using the fullReader is essential here!
		direction: direction,
		state:     state.Handshake,
		registry:  state.FromDirection(direction, state.Handshake, proto.MinimumVersion.Protocol),
		log:       log,
	}
}

type fullReader struct{ io.Reader }

func (fr *fullReader) Read(p []byte) (n int, err error) {
	n, err = io.ReadFull(fr.Reader, p)
	return
}

func (d *Decoder) SetState(state *state.Registry) {
	d.mu.Lock()
	d.state = state
	d.setProtocol(d.registry.Protocol)
	d.mu.Unlock()
}

func (d *Decoder) SetProtocol(protocol proto.Protocol) {
	d.mu.Lock()
	d.setProtocol(protocol)
	d.mu.Unlock()
}

func (d *Decoder) setProtocol(protocol proto.Protocol) {
	d.registry = state.FromDirection(d.direction, d.state, protocol)
}

func (d *Decoder) SetReader(rd io.Reader) {
	d.mu.Lock()
	d.rd = rd
	d.mu.Unlock()
}

func (d *Decoder) SetCompressionThreshold(threshold int) {
	d.mu.Lock()
	d.compressionThreshold = threshold
	d.compression = threshold >= 0
	d.mu.Unlock()
}

// ReadPacket blocks Decoder's mutex when the next packet's frame is known
// and stays blocked until the full packet from the underlying io.Reader is read.
//
// This is to ensure the mutex is not locked while being blocked by the first Read call
// (e.g. underlying io.Reader is a net.Conn).
func (d *Decoder) ReadPacket() (ctx *proto.PacketContext, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.readPacket()
}

func (d *Decoder) readPacket() (ctx *proto.PacketContext, err error) {
	payload, err := d.readPayload()
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		// Got an empty packet, skipping it
		return d.readPacket()
	}
	return d.decodePayload(payload)
}

// can eventually receive an empty payload which packet should be skipped
func (d *Decoder) readPayload() (payload []byte, err error) {
	payload, err = readVarIntFrame(d.rd)
	if err != nil {
		return
	}
	if len(payload) == 0 {
		return
	}
	if d.compression { // Decoder expects compressed payload
		// buf contains: claimedUncompressedSize + (compressed packet id & data)
		buf := bytes.NewBuffer(payload)
		claimedUncompressedSize, err := util.ReadVarInt(buf)
		if err != nil {
			return nil, err
		}
		if claimedUncompressedSize <= 0 {
			// This message is not compressed
			return buf.Bytes(), nil
		}
		return d.decompress(claimedUncompressedSize, buf)
	}
	return
}

func readVarIntFrame(rd io.Reader) (payload []byte, err error) {
	length, err := util.ReadVarInt(rd)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return // function caller should skip over empty packet
	}
	if length < 0 || length > 1048576 { // 2^(21-1)
		return nil, fmt.Errorf("received invalid packet length %d", length)
	}

	payload = make([]byte, length)
	_, err = rd.Read(payload)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (d *Decoder) decompress(claimedUncompressedSize int, rd io.Reader) (decompressed []byte, err error) {
	if claimedUncompressedSize < d.compressionThreshold {
		return nil, errs.NewSilentErr("uncompressed size %d is less than set threshold %d",
			claimedUncompressedSize, d.compressionThreshold)
	}
	if claimedUncompressedSize > UncompressedCap {
		return nil, errs.NewSilentErr("uncompressed size %d exceeds hard threshold of %d",
			claimedUncompressedSize, UncompressedCap)
	}

	z, err := zlib.NewReader(rd)
	if err != nil {
		return nil, err
	}

	// decompress payload
	decompressed = make([]byte, claimedUncompressedSize)
	_, err = io.ReadFull(z, decompressed)
	if err != nil {
		return nil, err
	}
	return decompressed, z.Close()
}

// Indicates a packet was known and successfully decoded by it's registered decoder,
// but the decoder has not read all of the packet's data.
//
// This may happen in cases where
//  - the decoder has a bug
//  - the decoder does not handle the case for the new protocol version of the packet changed by Mojang/Minecraft
//  - someone (server/client) has sent valid bytes in the beginning of the packet's data that the packet's
//    decoder could successfully decode, but then the data contains even more bytes (the left bytes)
var ErrDecoderLeftBytes = errors.New("decoder dis not read all packet data")

// DecodePayload takes p as the packet's payload that contains the packet id + data
// and returns a PacketContext that is the result of the decoding or returns an error.
//
// As a special case, decide whether you want to ignore the error ErrDecoderLeftBytes,
// that is returned when the payload's data had more bytes then the decoder has read,
// or drop the packet.
func (d *Decoder) decodePayload(p []byte) (ctx *proto.PacketContext, err error) {
	ctx = &proto.PacketContext{
		Direction:   d.direction,
		Protocol:    d.registry.Protocol,
		KnownPacket: false,
		Payload:     p,
	}
	payload := bytes.NewReader(p)

	// Read packet id.
	packetID, err := util.ReadVarInt(payload)
	if err != nil {
		return nil, err
	}
	ctx.PacketID = proto.PacketID(packetID)
	// Now the payload reader should only have left the packet's actual data.

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
		return ctx, ErrDecoderLeftBytes
	}

	// Packet decoder has read exactly all data from the payload.
	return
}
