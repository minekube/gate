package protoutil

import (
	"net"

	"github.com/pires/go-proxyproto"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Protocol returns the protocol version of the given subject if provided.
func Protocol(subject any) (proto.Protocol, bool) {
	// this method is implemented by proxy player
	if p, ok := subject.(interface{ Protocol() proto.Protocol }); ok {
		return p.Protocol(), true
	}
	return version.Unknown.Protocol, false
}

// ProxyHeader returns a proxy header for the given address.
func ProxyHeader(srcAddr, destAddr net.Addr) *proxyproto.Header {
	// Passing destination address as srcAddr fixes disconnect error
	// where the backend only allows IPv6 but the client connected with IPv4 (or vice-versa).
	// This assumes that the srcAddr uses the same protocol (e.g. TCP) as the destAddr.
	header := proxyproto.HeaderProxyFromAddrs(0, destAddr, destAddr)
	header.SourceAddr = srcAddr
	return header
}
