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

// UpdateBytesWritten updates the total bytes written counter.
// This is used internally for connection metrics.
UpdateBytesWritten(n int)
}

// NewWriter returns a new packet writer.
func NewWriter(conn net.Conn, direction proto.Direction, writeTimeout time.Duration, compressionLevel int, log logr.Logger, updateBytes func(int)) Writer {
writeBuf := bufio.NewWriter(conn)
return &writer{
log:              log.WithName("writer"),
writeTimeout:     writeTimeout,
compressionLevel: compressionLevel,
c:                conn,
writeBuf:         writeBuf,
Encoder:          codec.NewEncoder(writeBuf, direction, log.V(2)),
updateBytes:      updateBytes,
}
}

type writer struct {
log              logr.Logger
writeTimeout     time.Duration
compressionLevel int
c                net.Conn // underlying connection
writeBuf         *bufio.Writer
*codec.Encoder
updateBytes      func(int) // Callback to update connection's bytes written counter
}

func (w *writer) UpdateBytesWritten(n int) {
if w.updateBytes != nil {
w.updateBytes(n)
}
}

func (w *writer) Write(payload []byte) (n int, err error) {
n, err = w.Encoder.Write(payload)
w.UpdateBytesWritten(n)
return n, err
}

func (w *writer) WritePacket(packet proto.Packet) (n int, err error) {
n, err = w.Encoder.WritePacket(packet)
w.UpdateBytesWritten(n)
return n, err
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
