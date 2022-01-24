package tunnel

import (
	"bytes"
	pb "go.minekube.com/gate/pkg/gate/proto/tunnel/pb"
	"io"
	"net"
	"os"
	"time"
)

// conn implements net.Conn for a tunneled connection over gRPC
type conn struct {
	sessionID  string
	localAddr  net.Addr
	remoteAddr net.Addr

	rw      *deadlineReadWriter
	closeFn func(err error)
	err     error

	writeTimeout  time.Time
	readTimeout   time.Time
	writeRequests chan<- *writeRequest
	reads         <-chan []byte
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
	writeRequests := make(chan *writeRequest, 1)
	reads := make(chan []byte, 1)
	c := &conn{
		sessionID:     sessionID,
		localAddr:     localAddr,
		remoteAddr:    remoteAddr,
		rw:            newDeadlineReaderWriter(biStream),
		writeRequests: writeRequests,
		reads:         reads,
	}
	c.closeFn = func(err error) {
		c.err = err
		c.rw.Close()
		close(writeRequests)
		closeFn(err)
	}
	// read worker
	go func() {
		msg := new(pb.TunnelRequest) // reuse this struct
		for {
			if err := c.stream.RecvMsg(msg); err != nil {
				c.closeFn(err)
				return
			}
			reads <- msg.GetData()
		}
	}()
	// write worker
	go func() {
		for {
			req, ok := <-writeRequests
			if !ok {
				return
			}
			err := c.stream.Send(&pb.TunnelResponse{Data: req.data})
			req.response.n = len(req.data)
			req.data = nil
			if err != nil {
				req.response.n = 0
				req.response.err = err
				req.responseChan <- req
				continue
			}
			req.responseChan <- req
		}
	}()
	return c
}

var _ net.Conn = (*conn)(nil)

// SessionID returns the session id of the tunnel of this connection.
func (c *conn) SessionID() string { return c.sessionID }

func (c *conn) Read(b []byte) (n int, err error) { return c.rd.Read(b) }
func (c *conn) Write(b []byte) (n int, err error) {
	if c.err != nil {
		return 0, c.err
	}
	if time.Until(c.writeTimeout) <= 0 {
		return 0, os.ErrDeadlineExceeded
	}
	select {
	case <-time.After(time.Until(c.writeTimeout)):

	}
	responseChan := make(chan *writeRequest)
	select {
	case c.writeRequests <- &writeRequest{data: b, responseChan: responseChan}:
	case <-time.:
		return 0, c.err
	}
	select {
	case <-c.errChan:
		return 0, c.err
	case result, ok := <-responseChan:
		if !ok {
			return 0, c.err
		}
		return result.response.n, result.response.err
	}
}
func (c *conn) Close() error                       { c.closeFn(nil); return nil }
func (c *conn) LocalAddr() net.Addr                { return c.localAddr }
func (c *conn) RemoteAddr() net.Addr               { return c.remoteAddr }
func (c *conn) SetDeadline(t time.Time) error      { return c.setDeadline(t) }
func (c *conn) SetReadDeadline(t time.Time) error  { return c.setReadDeadline(t) }
func (c *conn) SetWriteDeadline(t time.Time) error { return c.setWriteDeadline(t) }

type deadlineReadWriter struct {
	stream  pb.TunnelService_TunnelServer
	readBuf bytes.Buffer
}

func newDeadlineReaderWriter(stream pb.TunnelService_TunnelServer) *deadlineReadWriter {

}

func (d *deadlineReadWriter) Read(p []byte) (n int, err error) {
	if c.readBuf.Len() < len(b) {
		msg := new(pb.TunnelRequest) // reuse this struct
		// More data requested
		for c.readBuf.Len() < len(b) {

			if err = c.stream.RecvMsg(msg); err != nil {
				c.closeFn(err)
				return
			}
			readChan <- msg.GetData()

			select {
			case <-c.errChan:
				return 0, c.err
			case data, ok := <-c.readChan:
				if !ok {
					return 0, c.err
				}
				_, _ = c.readBuf.Write(data)
			}
		}
	}
	return c.readBuf.Read(b)
}

var _ io.Reader = (*deadlineReadWriter)(nil)
