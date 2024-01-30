package protoutil

import (
	"fmt"
	"go.minekube.com/gate/pkg/util/netutil"
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
	srcAddr = convert(srcAddr)
	destAddr = convert(destAddr)

	header := proxyproto.HeaderProxyFromAddrs(0, srcAddr, destAddr)

	mismatch := func(srcIP, destIP net.IP) bool {
		// on mismatch v4 to v6: use v6
		return len(srcIP.To4()) == net.IPv4len && len(destIP) == net.IPv6len
	}

	switch sourceAddr := header.SourceAddr.(type) {
	case *net.TCPAddr:
		dstAddr, ok := destAddr.(*net.TCPAddr)
		if ok && mismatch(sourceAddr.IP, dstAddr.IP) {
			header.TransportProtocol = proxyproto.TCPv6
			sourceAddr.IP = sourceAddr.IP.To16()
			header.SourceAddr = sourceAddr
		}
	case *net.UDPAddr:
		dstAddr, ok := header.DestinationAddr.(*net.UDPAddr)
		if ok && mismatch(sourceAddr.IP, dstAddr.IP) {
			header.TransportProtocol = proxyproto.UDPv6
			sourceAddr.IP = sourceAddr.IP.To16()
			header.SourceAddr = sourceAddr
		}
	}
	return header
}

func convert(addr net.Addr) net.Addr {
	if addr == nil {
		return nil
	}
	switch addr.(type) {
	case *net.UDPAddr, *net.UnixAddr, *net.IPAddr, *net.TCPAddr:
		// fast path
		return addr
	default:
		// slow path
		host, port := netutil.HostPort(addr)
		ip := net.ParseIP(host)
		if ip == nil {
			err := fmt.Errorf("invalid IP address %T: %+v (host: %s, port: %d)", addr, addr, host, port)
			panic(err)
		}
		return &net.TCPAddr{
			IP:   ip,
			Port: int(port),
		}
	}
}
