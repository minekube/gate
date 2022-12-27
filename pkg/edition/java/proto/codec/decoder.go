package codec

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
)

// Decoder is a synchronized packet decoder
// for the Minecraft Java edition.
type Decoder struct {
	log       logr.Logger
	hexDump   bool // for debugging
	direction proto.Direction

	mu                   sync.Mutex // Protects following field and locked while reading a packet.
	rd                   io.Reader  // The underlying reader.
	registry             *state.ProtocolRegistry
	state                *state.Registry
	compression          bool
	compressionThreshold int
	zrd                  io.ReadCloser
}

var _ proto.PacketDecoder = (*Decoder)(nil)

func NewDecoder(r io.Reader, direction proto.Direction, log logr.Logger) *Decoder {
	return &Decoder{
		rd:        &fullReader{r}, // using the fullReader is essential here!
		direction: direction,
		state:     state.Handshake,
		registry:  state.FromDirection(direction, state.Handshake, version.MinimumVersion.Protocol),
		log:       log.WithName("decoder"),
		hexDump:   os.Getenv("HEXDUMP") == "true",
	}
}

type fullReader struct{ io.Reader }

func (fr *fullReader) Read(p []byte) (int, error) { return io.ReadFull(fr.Reader, p) }

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

// Decode reads the next packet from the underlying reader.
// It blocks other calls to Decode until return.
func (d *Decoder) Decode() (ctx *proto.PacketContext, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.readPacket()
}

func (d *Decoder) readPacket() (ctx *proto.PacketContext, err error) {
	if d.log.Enabled() { // check enabled for performance reason
		defer func() {
			if ctx != nil && ctx.KnownPacket() {
				d.log.Info("decoded packet", "context", ctx.String())
				if d.hexDump {
					fmt.Println(hex.Dump(ctx.Payload))
				}
			}
		}()
	}

retry:
	payload, err := d.readPayload()
	if err != nil {
		return nil, &errs.SilentError{Err: err}
	}
	if len(payload) == 0 {
		// Got an empty packet, skipping it
		goto retry
	}
	return d.decodePayload(payload)
}

// can eventually receive an empty payload which packet should be skipped
func (d *Decoder) readPayload() (payload []byte, err error) {
	payload, err = readVarIntFrame(d.rd)
	if err != nil {
		return nil, fmt.Errorf("error reading packet frame: %w", err)
	}
	if len(payload) == 0 {
		return
	}
	if d.compression { // Decoder expects compressed payload
		// buf contains: claimedUncompressedSize + (compressed packet id & data)
		buf := bytes.NewBuffer(payload)
		claimedUncompressedSize, err := util.ReadVarInt(buf)
		if err != nil {
			return nil, fmt.Errorf("error reading claimed uncompressed size varint: %w", err)
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
		return nil, fmt.Errorf("error reading varint: %w", err)
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
		return nil, fmt.Errorf("error reading payload: %w", err)
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

	if d.zrd == nil {
		d.zrd, err = zlib.NewReader(rd)
		if err != nil {
			return nil, err
		}
	} else {
		// Reuse already allocated zlib reader
		if err = d.zrd.(zlib.Resetter).Reset(rd, nil); err != nil {
			return nil, fmt.Errorf("error reseting zlib reader: %w", err)
		}
	}

	// decompress payload
	decompressed = make([]byte, claimedUncompressedSize)
	_, err = io.ReadFull(d.zrd, decompressed)
	if err != nil {
		return nil, fmt.Errorf("error decompressing payload: %w", err)
	}
	return decompressed, d.zrd.Close()
}

// DecodePayload takes p as the packet's payload that contains the packet id + data
// and returns a PacketContext that is the result of the decoding or returns an error.
//
// As a special case, decide whether you want to ignore the error ErrDecoderLeftBytes,
// that is returned when the payload's data had more bytes than the decoder has read,
// or drop the packet.
func (d *Decoder) decodePayload(p []byte) (ctx *proto.PacketContext, err error) {
	ctx = &proto.PacketContext{
		Direction: d.direction,
		Protocol:  d.registry.Protocol,
		Payload:   p,
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
	if err = ctx.Packet.Decode(ctx, payload); err != nil {
		if err == io.EOF { // payload was too short or decoder has a bug
			err = io.ErrUnexpectedEOF
		}
		return ctx, errs.NewSilentErr("error decoding packet (type: %T, id: %s, protocol: %s, direction: %s): %w",
			ctx.Packet, ctx.PacketID, ctx.Protocol, ctx.Direction, err)
	}

	// Payload buffer should now be empty.
	if payload.Len() != 0 {
		// packet decoder did not read all the packet's data!
		d.log.Info("packet decoder did not read all of packet's data",
			"ctx", ctx,
			"decodedBytes", len(ctx.Payload),
			"unreadBytes", payload.Len())
		return ctx, proto.ErrDecoderLeftBytes
	}

	// Packet decoder has read exactly all data from the payload.
	return
}
