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
	"go.minekube.com/gate/pkg/internal/bufpool"
)

const (
	VanillaMaximumUncompressedSize = 8 * 1024 * 1024  // 8MiB
	HardMaximumUncompressedSize    = 16 * 1024 * 1024 // 16MiB
	UncompressedCap                = VanillaMaximumUncompressedSize
)

// disable calibration, since most minecraft packets size is smaller than 64 bytes
var pool = &bufpool.Pool{DisableCalibration: true}

// Encoder is a synchronized packet encoder.
type Encoder struct {
	direction proto.Direction
	log       logr.Logger
	hexDump   bool // for debugging

	mu          sync.Mutex // Protects following fields
	wr          io.Writer  // the underlying writer to write successfully encoded packet to
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
		log:       log,
		hexDump:   os.Getenv("HEXDUMP") == "true",
		wr:        w,
		direction: direction,
		registry:  state.FromDirection(direction, state.Handshake, version.MinimumVersion.Protocol),
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
	packetID, found := e.registry.PacketID(packet)
	if !found {
		return n, fmt.Errorf("packet id for type %T in protocol %s not registered in the %s state registry",
			packet, e.registry.Protocol, e.state)
	}

	buf := bytes.NewBuffer(pool.Get())
	defer func() { pool.Put(buf.Bytes()) }()

	_ = util.WriteVarInt(buf, int(packetID))

	ctx := &proto.PacketContext{
		Direction:   e.direction,
		Protocol:    e.registry.Protocol,
		KnownPacket: true,
		PacketID:    packetID,
		Packet:      packet,
		Payload:     nil,
	}

	if err = packet.Encode(ctx, buf); err != nil {
		return
	}

	if e.log.Enabled() { // check enabled for performance reason
		e.log.Info("encoded packet", "context", ctx.String(), "bytes", buf.Len())
		if e.hexDump {
			fmt.Println(hex.Dump(ctx.Payload))
		}
	}

	return e.writeBuf(buf) // packet id + data
}

// see https://wiki.vg/Protocol#Packet_format for details
func (e *Encoder) writeBuf(payload *bytes.Buffer) (n int, err error) {
	if e.compression.enabled {
		compressed := bytes.NewBuffer(pool.Get())
		defer func() { pool.Put(compressed.Bytes()) }()

		uncompressedSize := payload.Len()
		if uncompressedSize < e.compression.threshold {
			// Under the threshold, there is nothing to do.
			_ = util.WriteVarInt(compressed, 0) // indicate not compressed
			_, _ = payload.WriteTo(compressed)
		} else {
			_ = util.WriteVarInt(compressed, uncompressedSize) // data length
			if err = e.compress(payload.Bytes(), compressed); err != nil {
				return 0, err
			}
		}
		// uncompressed length + packet id + data
		payload = compressed
	}

	frame := bytes.NewBuffer(pool.Get())
	defer func() { pool.Put(frame.Bytes()) }()
	frame.Grow(payload.Len() + 4)

	_ = util.WriteVarInt(frame, payload.Len()) // packet length
	_, _ = payload.WriteTo(frame)              // body

	m, err := frame.WriteTo(e.wr)
	return int(m), err
}

// Write encodes payload and writes it to the underlying writer.
// The payload must not already be compressed nor encrypted and must
// start with the packet's id VarInt and then the packet's data.
func (e *Encoder) Write(payload []byte) (n int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.writeBuf(bytes.NewBuffer(payload))
}

func (e *Encoder) compress(payload []byte, w io.Writer) (err error) {
	e.compression.writer.Reset(w)
	_, err = e.compression.writer.Write(payload)
	if err != nil {
		return err
	}
	return e.compression.writer.Close()
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
