package geyser

import (
	"errors"
	"net"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pires/go-proxyproto"
)

// wrapAcceptedConn applies Gate's PROXY protocol policy to a connection that
// was just accepted on the Geyser listener.
//
// The listener used to wrap every accepted connection with a bare
// proxyproto.NewConn, which is the library's REQUIRE default: an upstream that
// sent no header failed its first read with
// "proxyproto: proxy protocol signature not present" and the session was
// closed in the same second. Managed GeyserLite hit that on every join. The
// policy now matches the Java client listener - honour a header only from a
// trusted upstream, tolerate a peer that sends none - so an upstream that stops
// sending the header costs the Bedrock player's real client address instead of
// the whole Bedrock listener.
//
// The header is still parsed when the Java listener's own proxyProtocol switch
// is off: this listener only ever serves a Geyser process, and the Bedrock
// player's address has to survive the hop whether or not Gate itself is behind
// a PROXY protocol proxy.
func (i *Integration) wrapAcceptedConn(conn net.Conn) net.Conn {
	base := conn
	if i.proxy != nil {
		base = i.proxy.WrapProxyProtocolConn(conn)
	}
	listenAddr := ""
	if i.config != nil {
		listenAddr = i.config.GeyserListenAddr
	}
	return &proxyProtocolCauseConn{Conn: base, log: i.log, listenAddr: listenAddr}
}

// proxyProtocolCauseConn reports, once per connection, why a PROXY protocol
// header was rejected.
//
// Without it the decision made here surfaces only as a bare varint error in the
// Java client reader ("error reading varint: proxyproto: ..."), which names the
// symptom and not the cause: an upstream address missing from
// proxyProtocolTrustedProxies, or a Geyser that stopped sending the header.
type proxyProtocolCauseConn struct {
	net.Conn
	log        logr.Logger
	listenAddr string
	once       sync.Once
}

func (c *proxyProtocolCauseConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if err != nil {
		c.once.Do(func() { c.reportCause(err) })
	}
	return n, err
}

func (c *proxyProtocolCauseConn) reportCause(err error) {
	switch {
	case errors.Is(err, proxyproto.ErrSuperfluousProxyHeader):
		c.log.Info("geyser listener: rejected PROXY protocol header from an untrusted upstream",
			"cause", "the upstream sent a PROXY protocol header but its address is not covered by proxyProtocolTrustedProxies; "+
				"add the Geyser upstream to proxyProtocolTrustedProxies, or stop it sending a header (advanced.java.use-haproxy-protocol)",
			"listenAddr", c.listenAddr, "error", err)
	case errors.Is(err, proxyproto.ErrNoProxyProtocol):
		c.log.Info("geyser listener: PROXY protocol header missing while required",
			"cause", "the upstream sent no PROXY protocol header while this listener required one; "+
				"enable advanced.java.use-haproxy-protocol in the Geyser config",
			"listenAddr", c.listenAddr, "error", err)
	}
}
