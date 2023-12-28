package codec

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

const (
	VanillaMaximumUncompressedSize = 8 * 1024 * 1024   // 8MiB
	HardMaximumUncompressedSize    = 128 * 1024 * 1024 // 128MiB
	UncompressedCap                = VanillaMaximumUncompressedSize
)

// Encoder is a synchronized packet encoder.
type Encoder struct {
	direction proto.Direction
	log       logr.Logger
	hexDump   bool // for debugging

	mu          sync.Mutex // Protects following fields
	wr          io.Writer  // the underlying writer to write successfully encoded packets to
	registry    *state.ProtocolRegistry
	state       *state.Registry
	compression struct {
		enabled   bool
		threshold int // No compression if <= 0
		writer    *zlib.Writer
	}
}

func NewEncoder(w io.Writer, direction proto.Direction, log logr.Logger) *Encoder {
	return &Encoder{
		log:       log.WithName("encoder"),
		hexDump:   os.Getenv("HEXDUMP") == "true",
		wr:        w,
		direction: direction,
		registry:  state.FromDirection(direction, state.Handshake, version.MinimumVersion.Protocol),
		state:     state.Handshake,
	}
}

// Direction returns the encoder's direction.
func (e *Encoder) Direction() proto.Direction {
	return e.direction
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
	packetID, found := e.registry.PacketID(packet)
	if !found {
		return n, fmt.Errorf("packet id for type %T in protocol %s not registered in the %s %s state registry",
			packet, e.registry.Protocol, e.direction, e.state)
	}

	pk := reflect.TypeOf(packet)
	buf, release := encodePool.getBuf(pk)
	defer release()

	_ = util.WriteVarInt(buf, int(packetID))

	ctx := &proto.PacketContext{
		Direction: e.direction,
		Protocol:  e.registry.Protocol,
		PacketID:  packetID,
		Packet:    packet,
		Payload:   nil,
	}

	if err = util.RecoverFunc(func() error {
		return packet.Encode(ctx, buf)
	}); err != nil {
		return
	}

	if e.log.Enabled() { // check enabled for performance reason
		e.log.Info("encoded packet", "context", ctx.String(), "bytes", buf.Len())
		if e.hexDump {
			fmt.Println(hex.Dump(ctx.Payload))
		}
	}

	return e.writeBuf(buf, pk) // packet id + data
}

// see https://wiki.vg/Protocol#Packet_format for details
func (e *Encoder) writeBuf(payload *bytes.Buffer, pk reflect.Type) (n int, err error) {
	if e.compression.enabled {
		return e.writeCompressed(payload, pk.String())
	}
	n, err = util.WriteVarIntN(e.wr, payload.Len()) // packet length
	if err != nil {
		return n, err
	}
	m, err := payload.WriteTo(e.wr) // body
	return int(m) + n, err
}

func (e *Encoder) writeCompressed(payload *bytes.Buffer, pk any) (n int, err error) {
	uncompressedSize := payload.Len()
	if uncompressedSize < e.compression.threshold {
		// Under the threshold, there is nothing to do.
		n, err = util.WriteVarIntN(e.wr, uncompressedSize+1) // packet length
		if err != nil {
			return n, err
		}
		n2, err := util.WriteVarIntN(e.wr, 0) // indicate not compressed
		if err != nil {
			return n + n2, err
		}
		n3, err := payload.WriteTo(e.wr) // body
		return n + n2 + int(n3), err
	}
	// >= threshold, compress packet id + data

	compressed, release := compressPool.getBuf(pk)
	defer release()

	err = util.WriteVarInt(compressed, uncompressedSize) // data length
	if err != nil {
		return 0, err
	}
	_, err = e.compress(payload.Bytes(), compressed)
	if err != nil {
		return 0, err
	}
	n, err = util.WriteVarIntN(e.wr, compressed.Len()) // packet length
	if err != nil {
		return n, err
	}
	m, err := compressed.WriteTo(e.wr) // body
	return n + int(m), err
}

// Write encodes payload and writes it to the underlying writer.
// The payload must not already be compressed nor encrypted and must
// start with the packet's id VarInt and then the packet's data.
func (e *Encoder) Write(payload []byte) (n int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.writeBuf(bytes.NewBuffer(payload), compressedKey)
}

var compressedKey = reflect.TypeOf((*complex128)(nil))

func (e *Encoder) compress(payload []byte, w io.Writer) (n int, err error) {
	e.compression.writer.Reset(w)
	n, err = e.compression.writer.Write(payload)
	if err != nil {
		return n, err
	}
	return n, e.compression.writer.Close()
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

// Sync locks the encoder while running fn,
// making sure no write calls are run during this call.
func (e *Encoder) Sync(fn func() error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return fn()
}
