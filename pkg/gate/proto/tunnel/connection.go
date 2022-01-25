package tunnel

import (
	pb "go.minekube.com/gate/pkg/gate/proto/tunnel/pb"
	"go.minekube.com/gate/pkg/internal/deadline"
	"net"
	"time"
)

// conn implements net.Conn for a tunneled connection over gRPC
type conn struct {
	sessionID  string
	localAddr  net.Addr
	remoteAddr net.Addr

	closeFn func(err error)
	w       deadline.Writer
	r       deadline.Reader
}

type writeRequest struct {
	data     []byte
	response struct {
		n   int
		err error
	}
	responseChan chan<- *writeRequest
}

func newConn(
	sessionID string,
	localAddr, remoteAddr net.Addr,
	biStream pb.TunnelService_TunnelServer,
	closeFn func(err error),
) *conn {
	c := &conn{
		sessionID:  sessionID,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
		w: deadline.NewWriter(func() deadline.WriteFn {
			msg := new(pb.TunnelResponse)
			return func(b []byte) (err error) {
				msg.Data = b
				return biStream.Send(msg)
			}
		}()),
		r: deadline.NewReader(func() deadline.ReadFn {
			msg := new(pb.TunnelRequest)
			return func() (b []byte, err error) {
				err = biStream.RecvMsg(msg)
				return msg.GetData(), err
			}
		}()),
	}
	c.closeFn = func(err error) {
		_ = c.w.Close()
		closeFn(err)
	}
	return c
}

var _ net.Conn = (*conn)(nil)

// SessionID returns the session id of the tunnel of this connection.
func (c *conn) SessionID() string { return c.sessionID }

func (c *conn) Read(b []byte) (n int, err error)  { return c.r.Read(b) }
func (c *conn) Write(b []byte) (n int, err error) { return c.w.Write(b) }
func (c *conn) Close() error                      { c.closeFn(nil); return nil }
func (c *conn) LocalAddr() net.Addr               { return c.localAddr }
func (c *conn) RemoteAddr() net.Addr              { return c.remoteAddr }
func (c *conn) SetDeadline(t time.Time) error {
	err := c.w.SetDeadline(t)
	if err != nil {
		return err
	}
	return c.r.SetDeadline(t)
}
func (c *conn) SetReadDeadline(t time.Time) error  { return c.r.SetDeadline(t) }
func (c *conn) SetWriteDeadline(t time.Time) error { return c.w.SetDeadline(t) }
