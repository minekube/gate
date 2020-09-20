package proxy

import (
	"bufio"
	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/runtime/logr"
	"io"
	"net"
	"sync"
)

type minecraftConn struct {
	proxy *Proxy
	log   logr.Logger
	c     net.Conn

	rwBuf   *bufio.ReadWriter
	decoder *codec.Decoder
	encoder *codec.Encoder

	mu sync.RWMutex // Protects following fields
	// state
	// sessionHandler
}

func newMinecraftConn(base net.Conn, proxy *Proxy, isPlayer bool) (conn *minecraftConn) {
	in := proto.ServerBound  // reads from client are server bound (proxy <- client)
	out := proto.ClientBound // writes to client are client bound (proxy -> client)
	logName := "player-conn"
	if !isPlayer { // if a backend server connection
		in = proto.ClientBound  // reads from backend are client bound (proxy <- backend)
		out = proto.ServerBound // writes to backend are server bound (proxy -> backend)
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
		decoder: codec.NewDecoder(rwBuf.Reader, in, log.WithName("decoder")),
		encoder: codec.NewEncoder(rwBuf.Writer, out, log.WithName("encoder")),
	}
}

func (c *minecraftConn) readLoop() {
	//defer c.close()

	//// Set read timeout to wait for client to send packet/s
	//deadline := time.Now().Add(time.Duration(c.config().ReadTimeout) * time.Millisecond)
	//_ = c.c.SetReadDeadline(deadline)
	for {
		packets, err := c.decoder.Decode()
		if err != nil {
			if err != io.EOF { // EOF means connection was closed
				c.log.V(1).Info("Error decoding next packets, closing connection", "err", err)
			}
			return
		}
	}
}

func (c *minecraftConn) config() *config.Config {
	return c.proxy.config
}
