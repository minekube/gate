package tunnel

import (
	"bytes"
	pb "go.minekube.com/gate/pkg/gate/proto/tunnel/pb"
	"net"
	"time"
)

// conn implements net.Conn for a tunneled connection over gRPC
type conn struct {
	closeTunnel func(err error)

	sessionID  string
	bi         pb.TunnelService_TunnelServer
	readBuf    bytes.Buffer
	localAddr  net.Addr
	remoteAddr net.Addr
}

var _ net.Conn = (*conn)(nil)

// SessionID returns the session id of the tunnel of this connection.
func (c *conn) SessionID() string { return c.sessionID }

func (c *conn) Read(b []byte) (n int, err error) {
	for c.readBuf.Len() < len(b) {
		// More data requested
		var msg *pb.TunnelRequest
		msg, err = c.bi.Recv()
		if err != nil {
			return 0, err
		}
		_, _ = c.readBuf.Write(msg.GetData())
	}
	return c.readBuf.Read(b)
}
func (c *conn) Write(b []byte) (n int, err error) {
	err = c.bi.Send(&pb.TunnelResponse{Data: b})
	if err != nil {
		return 0, err
	}
	return len(b), nil
}
func (c *conn) Close() error         { c.closeTunnel(nil); return nil }
func (c *conn) LocalAddr() net.Addr  { return c.localAddr }
func (c *conn) RemoteAddr() net.Addr { return c.remoteAddr }
func (c *conn) SetDeadline(t time.Time) error {
	// I don't think an implementation is necessary
	return nil
}
func (c *conn) SetReadDeadline(t time.Time) error {
	// I don't think an implementation is necessary
	return nil
}
func (c *conn) SetWriteDeadline(t time.Time) error {
	// I don't think an implementation is necessary
	return nil
}
