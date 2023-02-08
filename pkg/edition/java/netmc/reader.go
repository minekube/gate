package netmc

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
)

// Reader is a packet reader.
type Reader interface {
	// ReadPacket reads the next packet from the connection.
	// If the reader should retry reading the next packet, it returns ErrReadPacketRetry.
	// If the reader returns an error, it returns the connection is in a broken and should be closed.
	ReadPacket() (*proto.PacketContext, error)
	// ReadBuffered reads the remaining buffered bytes from the reader.
	// This is useful for emptying the Reader when it is not needed anymore.
	ReadBuffered() ([]byte, error)
	StateChanger
}

// ErrReadPacketRetry is returned by ReadPacket when the reader should retry reading the next packet.
var ErrReadPacketRetry = errors.New("error reading packet, retry")

// NewReader returns a new packet reader.
func NewReader(conn net.Conn, direction proto.Direction, readTimeout time.Duration, log logr.Logger) Reader {
	readBuf := bufio.NewReader(conn)
	return &reader{
		c:           conn,
		readTimeout: readTimeout,
		log:         log.WithName("reader"),
		readBuf:     readBuf,
		Decoder:     codec.NewDecoder(readBuf, direction, log.V(2)),
	}
}

type reader struct {
	log         logr.Logger
	readTimeout time.Duration
	c           net.Conn // underlying connection
	readBuf     *bufio.Reader
	*codec.Decoder
}

func (r *reader) ReadPacket() (*proto.PacketContext, error) {
	// Set read timeout to wait for client to send a packet
	_ = r.c.SetReadDeadline(time.Now().Add(r.readTimeout))

	packetCtx, err := r.Decode()
	if err != nil && !errors.Is(err, proto.ErrDecoderLeftBytes) { // Ignore this error.
		if r.handleReadErr(err) {
			r.log.V(1).Info("error reading packet, recovered", "error", err)
			return nil, ErrReadPacketRetry
		}
		r.log.V(1).Info("error reading packet, closing connection", "error", err)
		return nil, err
	}
	return packetCtx, nil
}

// handles error when read the next packet
func (r *reader) handleReadErr(err error) (recoverable bool) {
	var silentErr *errs.SilentError
	if errors.As(err, &silentErr) {
		return false
	}
	// Immediately retry for EAGAIN
	if errors.Is(err, syscall.EAGAIN) {
		return true
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Temporary() {
			// Immediately retry for temporary network errors
			return true
		} else if netErr.Timeout() {
			// Read timeout, disconnect
			r.log.Error(err, "read timeout")
			return false
		} else if errs.IsConnClosedErr(netErr.Err) {
			// Connection is already closed
			return false
		}
	}
	// Immediately break for known unrecoverable errors
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.Canceled) ||
		errors.Is(err, io.ErrNoProgress) || errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, io.ErrShortBuffer) || errors.Is(err, syscall.EBADF) ||
		strings.Contains(err.Error(), "use of closed file") {
		return false
	}
	r.log.Error(err, "error reading next packet, unrecoverable and closing connection")
	return false
}

func (r *reader) EnableEncryption(secret []byte) error {
	decryptReader, err := codec.NewDecryptReader(r.readBuf, secret)
	if err != nil {
		return err
	}
	r.Decoder.SetReader(decryptReader)
	return nil
}

func (r *reader) SetCompressionThreshold(threshold int) error {
	r.Decoder.SetCompressionThreshold(threshold)
	return nil
}

func (r *reader) ReadBuffered() ([]byte, error) {
	b := make([]byte, r.readBuf.Buffered())
	_, err := r.readBuf.Read(b)
	return b, err
}
