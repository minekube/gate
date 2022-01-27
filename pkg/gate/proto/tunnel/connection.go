package tunnel

import (
	pb "go.minekube.com/gate/pkg/gate/proto/tunnel/pb"
	"go.minekube.com/gate/pkg/internal/deadline"
	"net"
	"time"
)

// conn implements net.Conn for a tunneled connection over gRPC
type conn struct {
	s *pb.Session

	localAddr  net.Addr
	remoteAddr net.Addr

	closeFn func(err error)
	w       deadline.Writer
	r       deadline.Reader
}

func (c *conn) Session() *pb.Session { return c.s }

type Conn interface {
	net.Conn
	Session() *pb.Session
}

func serverStreamRW(ss pb.TunnelService_TunnelServer) (r deadline.Reader, w deadline.Writer) {
	return deadline.NewReader(func() ([]byte, error) { msg, err := ss.Recv(); return msg.GetData(), err }),
		deadline.NewWriter(func(b []byte) (err error) { return ss.Send(&pb.TunnelResponse{Data: b}) })
}

func clientStreamRW(ss pb.TunnelService_TunnelClient) (r deadline.Reader, w deadline.Writer) {
	return deadline.NewReader(func() ([]byte, error) { msg, err := ss.Recv(); return msg.GetData(), err }),
		deadline.NewWriter(func(b []byte) (err error) {
			return ss.Send(&pb.TunnelRequest{Message: &pb.TunnelRequest_Data{Data: b}})
		})
}

func newConn(
	session *pb.Session,
	localAddr, remoteAddr net.Addr,
	r deadline.Reader, w deadline.Writer,
	closeFn func(err error),
) Conn {
	c := &conn{
		s:          session,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
		w:          w,
		r:          r,
	}
	c.closeFn = func(err error) {
		_ = c.w.Close()
		closeFn(err)
	}
	return c
}

var _ net.Conn = (*conn)(nil)

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
