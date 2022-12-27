package proxy

import (
	"bufio"
	"io"
	"net"
	"sync"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/proto"
	protoutil "go.minekube.com/gate/pkg/gate/proto"
)

type sessionHandler interface {
	handlePacket(p *protoutil.PacketContext)
}

type minecraftConn struct {
	proxy *Proxy
	log   logr.Logger
	c     net.Conn

	rwBuf   *bufio.ReadWriter
	decoder *proto.Decoder
	encoder *proto.Encoder

	mu sync.RWMutex // Protects following fields
	// state
	// sessionHandler
}

func newMinecraftConn(base net.Conn, proxy *Proxy, isPlayer bool) (conn *minecraftConn) {
	in := protoutil.ServerBound  // reads from client are server bound (proxy <- client)
	out := protoutil.ClientBound // writes to client are client bound (proxy -> client)
	logName := "player-conn"
	if !isPlayer { // if a backend server connection
		in = protoutil.ClientBound  // reads from backend are client bound (proxy <- backend)
		out = protoutil.ServerBound // writes to backend are server bound (proxy -> backend)
		logName = "backend-conn"
	}

	log := proxy.log.WithName(logName).WithValues("remoteAddr", base.RemoteAddr())
	rwBuf := &bufio.ReadWriter{
		Reader: bufio.NewReader(base),
		Writer: bufio.NewWriter(base),
	}
	return &minecraftConn{
		proxy:   proxy,
		log:     log,
		c:       base,
		rwBuf:   rwBuf,
		decoder: proto.NewDecoder(rwBuf.Reader, in, log.WithName("decoder")),
		encoder: proto.NewEncoder(rwBuf.Writer, out, log.WithName("encoder")),
	}
}

func (c *minecraftConn) readLoop() {
	//defer c.close()

	//// Set read timeout to wait for client to send packet/s
	//deadline := time.Now().Add(time.Duration(c.config().ReadTimeout) * time.Millisecond)
	//_ = c.c.SetReadDeadline(deadline)
	for {
		packetCtx, err := c.decoder.Decode()
		if err != nil {
			if err != io.EOF { // EOF means connection was closed
				c.log.V(1).Info("Error decoding next packets, closing connection", "error", err)
			}
			return
		}
		// TODO
		_ = packetCtx
	}
}

func (c *minecraftConn) config() *config.Config {
	return c.proxy.config
}
