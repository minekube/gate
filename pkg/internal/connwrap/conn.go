package connwrap

import (
	"net"

	"go.uber.org/atomic"
)

// Conn is a wrapper around a net.Conn that tracks whether Closed has been called.
type Conn struct {
	net.Conn // underlying connection
	closed   atomic.Bool
}

func (c *Conn) Close() error {
	c.closed.Store(true)
	return c.Conn.Close()
}

func (c *Conn) Closed() bool {
	return c.closed.Load()
}
