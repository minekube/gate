package codec

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/state"
	"go.minekube.com/gate/pkg/proto/util"
	"io"
	"sync"
)

const (
	VanillaMaximumUncompressedSize = 2 * 1024 * 1024  // 2MiB
	HardMaximumUncompressedSize    = 16 * 1024 * 1024 // 16MiB
	UncompressedCap                = VanillaMaximumUncompressedSize
)

// Encoder is a synchronized packet encoder.
type Encoder struct {
	direction proto.Direction

	mu          sync.Mutex // Protects following fields
	wr          io.Writer  // to underlying writer to write successfully encoded packet to
	registry    *state.ProtocolRegistry
	state       *state.Registry
	compression struct {
		enabled   bool
		threshold int // No compression if <= 0
		writer    *zlib.Writer
	}
}

func NewEncoder(w io.Writer, direction proto.Direction) *Encoder {
	return &Encoder{
		wr:        w,
		direction: direction,
		registry:  state.FromDirection(direction, state.Handshake, proto.MinimumVersion.Protocol),
		state:     state.Handshake,
	}
}

func (e *Encoder) SetCompression(threshold, level int) (err error) {
	e.mu.Lock()
	e.compression.threshold = threshold
	e.compression.enabled = threshold >= 0
	if e.compression.enabled {
		e.compression.writer, err = zlib.NewWriterLevel(e.wr, level)
	}
	e.mu.Unlock()
	return
}

func (e *Encoder) WritePacket(packet proto.Packet) (n int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	packetId, found := e.registry.PacketId(packet)
	if !found {
		return n, fmt.Errorf("packet id for type %T in protocol %s not registered in the %s state registry",
			packet, e.registry.Protocol, e.state)
	}
	buf := new(bytes.Buffer)
	_ = util.WriteVarInt(buf, int(packetId))

	ctx := &proto.PacketContext{
		Direction:   e.direction,
		Protocol:    e.registry.Protocol,
		KnownPacket: true,
		PacketId:    packetId,
		Packet:      packet,
		Payload:     nil,
	}

	if err = packet.Encode(ctx, buf); err != nil {
		return
	}

	return e.writeBuf(buf) // packet id + data
}

// Write encodes payload (uncompressed and unencrypted) (containing packed id + data)
// and writes it to the underlying writer.
func (e *Encoder) WriteBuf(payload *bytes.Buffer) (n int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.writeBuf(payload)
}

// see https://wiki.vg/Protocol#Packet_format for details
func (e *Encoder) writeBuf(payload *bytes.Buffer) (n int, err error) {
	if e.compression.enabled {
		compressed := new(bytes.Buffer)
		uncompressedSize := payload.Len()
		if uncompressedSize <= e.compression.threshold {
			// Under the threshold, there is nothing to do.
			_ = util.WriteVarInt(compressed, 0)
			_, _ = payload.WriteTo(compressed)
		} else {
			_ = util.WriteVarInt(compressed, uncompressedSize)
			if err = e.compress(payload.Bytes(), compressed); err != nil {
				return 0, err
			}
		}
		// uncompressed length + packet id + data
		payload = compressed
	}

	frame := bytes.NewBuffer(make([]byte, 0, payload.Len()+5))
	_ = util.WriteVarInt(frame, payload.Len())
	_, _ = payload.WriteTo(frame)

	n = frame.Len()
	_, err = frame.WriteTo(e.wr)
	return
}

// Write encodes and writes the uncompressed and unencrypted payload (packed id + data).
func (e *Encoder) Write(payload []byte) (n int, err error) {
	return e.WriteBuf(bytes.NewBuffer(payload))
}

func (e *Encoder) compress(payload []byte, w io.Writer) (err error) {
	e.compression.writer.Reset(w)
	_, err = e.compression.writer.Write(payload)
	if err != nil {
		return err
	}
	return e.compression.writer.Flush()
}

func (e *Encoder) SetProtocol(protocol proto.Protocol) {
	e.mu.Lock()
	e.setProtocol(protocol)
	e.mu.Unlock()
}
func (e *Encoder) setProtocol(protocol proto.Protocol) {
	e.registry = state.FromDirection(e.direction, e.state, protocol)
}

func (e *Encoder) SetState(state *state.Registry) {
	e.mu.Lock()
	e.state = state
	e.setProtocol(e.registry.Protocol)
	e.mu.Unlock()
}

func (e *Encoder) SetWriter(w io.Writer) {
	e.mu.Lock()
	e.wr = w
	e.mu.Unlock()
}
