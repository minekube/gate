package proxy

import (
	"fmt"
	"net"
	"time"

	"github.com/pires/go-proxyproto"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/util/netutil"
)

// proxyProtocolReadHeaderTimeout bounds how long sniffing for a PROXY protocol
// header may wait for the client's first bytes.
//
// proxyproto.NewConn applies no deadline on its own (the 10s default only
// exists on proxyproto.Listener), so without this a client that connects and
// then sends nothing keeps a goroutine and file descriptor parked forever.
// On timeout the connection is simply treated as having no PROXY header and
// continues under Config.ReadTimeout like any other connection.
const proxyProtocolReadHeaderTimeout = 10 * time.Second

// proxyProtocol wraps accepted connections for PROXY protocol parsing, honoring
// the header only when it comes from a trusted upstream.
//
// Without a trusted upstream set, anybody able to open a TCP connection to the
// proxy could prepend a PROXY header and claim to connect from any IP address,
// which would let them evade an IP ban and get an innocent third party banned
// instead.
type proxyProtocol struct {
	trusted netutil.TrustedNetworks
}

// newProxyProtocol builds the PROXY protocol connection wrapper for cfg.
// It returns an error if the configured trusted upstreams are malformed.
func newProxyProtocol(cfg *config.Config) (*proxyProtocol, error) {
	trusted, err := netutil.ParseTrustedNetworks(
		config.ResolveProxyProtocolTrustedProxies(cfg.ProxyProtocolTrustedProxies))
	if err != nil {
		return nil, fmt.Errorf("invalid proxyProtocolTrustedProxies: %w", err)
	}
	return &proxyProtocol{trusted: trusted}, nil
}

// trustedNetworks returns the upstreams allowed to send a PROXY protocol
// header. A nil proxyProtocol trusts no upstream, so an unset wrapper fails
// closed instead of honoring forged headers.
func (p *proxyProtocol) trustedNetworks() netutil.TrustedNetworks {
	if p == nil {
		return nil
	}
	return p.trusted
}

// wrapConn wraps conn so that a PROXY protocol header sent by a trusted
// upstream rewrites the connection's remote address.
//
// A header from any other peer is rejected: reading from the returned
// connection fails, so an untrusted client can neither assert an address nor
// have its forged header silently stripped. Untrusted peers that send no header
// - every regular Minecraft client - are unaffected and pass through.
func (p *proxyProtocol) wrapConn(conn net.Conn) net.Conn {
	return p.wrapConnTimeout(conn, proxyProtocolReadHeaderTimeout)
}

func (p *proxyProtocol) wrapConnTimeout(conn net.Conn, readHeaderTimeout time.Duration) net.Conn {
	policy := proxyproto.REJECT
	if p.trustedNetworks().Contains(conn.RemoteAddr()) {
		policy = proxyproto.USE
	}
	return proxyproto.NewConn(conn,
		proxyproto.WithPolicy(policy),
		proxyproto.SetReadHeaderTimeout(readHeaderTimeout),
	)
}
