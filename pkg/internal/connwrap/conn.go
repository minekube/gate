package connwrap

import (
	"net"

	connectiontelemetry "go.minekube.com/gate/pkg/telemetry/connection"
	"go.uber.org/atomic"
)

// Conn is a wrapper around a net.Conn that tracks whether Close has been called.
type Conn struct {
	net.Conn // underlying connection
	closed   atomic.Bool
}

func (c *Conn) Close() error {
	c.closed.Store(true)
	return c.Conn.Close()
}

// CloseWrite preserves TCP half-close semantics for a connection that has
// passed through ConnectionEvent's close-tracking wrapper.
func (c *Conn) CloseWrite() error {
	if conn, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return conn.CloseWrite()
	}
	return net.ErrClosed
}

func (c *Conn) Closed() bool {
	return c.closed.Load()
}

// TelemetryWireConn forwards the raw-wire counter through ConnectionEvent's
// close-tracking wrapper, so event subscribers cannot accidentally erase the
// PROXY-header accounting boundary by retaining the original connection.
func (c *Conn) TelemetryWireConn() *connectiontelemetry.TrackedConn {
	if carrier, ok := c.Conn.(interface {
		TelemetryWireConn() *connectiontelemetry.TrackedConn
	}); ok {
		return carrier.TelemetryWireConn()
	}
	return nil
}
