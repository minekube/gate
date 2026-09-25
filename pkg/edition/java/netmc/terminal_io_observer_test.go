package netmc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type terminalIOObservation struct {
	direction string
	class     string
	observed  int
	limit     int
}

type terminalObservingConn struct {
	net.Conn
	mu     sync.Mutex
	events []terminalIOObservation
}

func (c *terminalObservingConn) ObserveMinecraftTerminalIO(direction, class string, observed, limit int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, terminalIOObservation{direction: direction, class: class, observed: observed, limit: limit})
}

func (c *terminalObservingConn) observations() []terminalIOObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]terminalIOObservation(nil), c.events...)
}

func TestReaderReportsBoundedTerminalIOClass(t *testing.T) {
	tests := []struct {
		name      string
		wire      []byte
		wantClass string
		observed  int
		limit     int
	}{
		{
			name:      "ordinary EOF",
			wantClass: "transport_closed",
		},
		{
			name:      "decoder failure",
			wire:      []byte{6, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}, // packet-id VarInt exceeds its wire bound
			wantClass: "decode_error",
		},
		{
			name:      "frame too large",
			wire:      frameLengthPrefix(codec.MaximumFrameLength + 1),
			wantClass: "frame_too_large",
			observed:  codec.MaximumFrameLength + 1,
			limit:     codec.MaximumFrameLength,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local, remote := net.Pipe()
			t.Cleanup(func() { _ = local.Close() })
			observed := &terminalObservingConn{Conn: local}
			go func() {
				if len(tt.wire) > 0 {
					_, _ = remote.Write(tt.wire)
				}
				_ = remote.Close()
			}()

			_, err := NewReader(observed, proto.ClientBound, time.Second, logr.Discard()).ReadPacket()
			require.Error(t, err)
			require.Equal(t, []terminalIOObservation{{
				direction: "read",
				class:     tt.wantClass,
				observed:  tt.observed,
				limit:     tt.limit,
			}}, observed.observations())
		})
	}
}

func TestProtocol758FrameBoundaryReportsOverLimit(t *testing.T) {
	// Java 1.18.2 is protocol 758. The maximum frame is enforced before
	// packet-registry decoding, so this harness isolates the exact wire boundary
	// used by that client generation without relying on WebSocket message size.
	require.Equal(t, 758, int(version.Minecraft_1_18_2.Protocol))

	local, remote := net.Pipe()
	defer local.Close()
	observed := &terminalObservingConn{Conn: local}
	go func() {
		var wire bytes.Buffer
		_ = util.WriteVarInt(&wire, codec.MaximumFrameLength+1)
		_, _ = remote.Write(wire.Bytes())
		_ = remote.Close()
	}()

	_, err := NewReader(observed, proto.ServerBound, time.Second, logr.Discard()).ReadPacket()
	require.Error(t, err)
	require.Equal(t, []terminalIOObservation{{
		direction: "read",
		class:     "frame_too_large",
		observed:  codec.MaximumFrameLength + 1,
		limit:     codec.MaximumFrameLength,
	}}, observed.observations())
}

func TestWriterReportsBoundedTerminalIOClassWithoutRawError(t *testing.T) {
	raw := errors.New("write to private.example (203.0.113.8) failed")
	base := &terminalObservingConn{Conn: &writeFailConn{err: raw}}
	conn, _ := NewMinecraftConn(context.Background(), base, proto.ClientBound, time.Second, time.Second, -1, nil)

	err := conn.Write([]byte{0})
	require.ErrorIs(t, err, raw)
	require.Equal(t, []terminalIOObservation{{direction: "write", class: "write_failure"}}, base.observations())
}

func frameLengthPrefix(length int) []byte {
	var frame bytes.Buffer
	_ = util.WriteVarInt(&frame, length)
	return frame.Bytes()
}

type writeFailConn struct{ err error }

func (c *writeFailConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *writeFailConn) Write([]byte) (int, error)        { return 0, c.err }
func (c *writeFailConn) Close() error                     { return nil }
func (c *writeFailConn) LocalAddr() net.Addr              { return terminalTestAddr("local") }
func (c *writeFailConn) RemoteAddr() net.Addr             { return terminalTestAddr("remote") }
func (c *writeFailConn) SetDeadline(time.Time) error      { return nil }
func (c *writeFailConn) SetReadDeadline(time.Time) error  { return nil }
func (c *writeFailConn) SetWriteDeadline(time.Time) error { return nil }

type terminalTestAddr string

func (a terminalTestAddr) Network() string { return "test" }
func (a terminalTestAddr) String() string  { return string(a) }
