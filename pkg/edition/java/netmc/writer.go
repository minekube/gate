package netmc

import (
	"bufio"
	"net"
	"time"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Writer is a packet writer.
type Writer interface {
	// WritePacket writes a packet to the connection's write buffer.
	WritePacket(packet proto.Packet) (n int, err error)
	// Write encodes payload and writes it to the underlying writer.
	// The payload must not already be compressed nor encrypted and must
	// start with the packet's id VarInt and then the packet's data.
	Write(payload []byte) (n int, err error)
	// Flush flushes the connection's write buffer.
	Flush() (err error)

	StateChanger
	Direction() proto.Direction
}

// NewWriter returns a new packet writer.
func NewWriter(conn net.Conn, direction proto.Direction, writeTimeout time.Duration, compressionLevel int, log logr.Logger) Writer {
	writeBuf := bufio.NewWriter(conn)
	return &writer{
		log:              log.WithName("writer"),
		writeTimeout:     writeTimeout,
		compressionLevel: compressionLevel,
		c:                conn,
		writeBuf:         writeBuf,
		Encoder:          codec.NewEncoder(writeBuf, direction, log.V(2)),
	}
}

type writer struct {
	log              logr.Logger
	writeTimeout     time.Duration
	compressionLevel int
	c                net.Conn // underlying connection
	writeBuf         *bufio.Writer
	*codec.Encoder
}

func (w *writer) Flush() (err error) {
	// Handle err in case the connection is
	// already closed and can't write to.
	if err = w.c.SetWriteDeadline(time.Now().Add(w.writeTimeout)); err != nil {
		return err
	}
	// Must flush in sync with encoder, or we may get an
	// io.ErrShortWrite when flushing while encoder is already writing.
	return w.Encoder.Sync(w.writeBuf.Flush)
}

func (w *writer) SetCompressionThreshold(threshold int) error {
	return w.Encoder.SetCompression(threshold, w.compressionLevel)
}

func (w *writer) EnableEncryption(secret []byte) error {
	encryptWriter, err := codec.NewEncryptWriter(w.writeBuf, secret)
	if err != nil {
		return err
	}
	w.Encoder.SetWriter(encryptWriter)
	return nil
}
