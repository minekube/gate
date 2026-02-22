package lite

import (
	"net"
	"net/netip"
	"strings"

	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/util/netutil"
)

func effectiveClientIP(route *liteconfig.Route, handshakeHost string, clientAddr net.Addr) netip.Addr {
	if route != nil && route.GetTCPShieldRealIP() && IsTCPShieldRealIP(handshakeHost) {
		if ip, ok := tcpShieldClientIP(handshakeHost); ok {
			return ip
		}
	}
	if clientAddr == nil {
		return netip.Addr{}
	}
	host := netutil.Host(clientAddr)
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return ip
}

func tcpShieldClientIP(host string) (netip.Addr, bool) {
	parts := strings.Split(host, tcpShieldRealIPSeparator)
	if len(parts) < 2 {
		return netip.Addr{}, false
	}
	clientPart := strings.Split(parts[1], forgeSeparator)[0]
	if clientPart == "" {
		return netip.Addr{}, false
	}
	if hostOnly := netutil.HostStr(clientPart); hostOnly != "" && hostOnly != clientPart {
		clientPart = hostOnly
	}
	ip, err := netip.ParseAddr(clientPart)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip, true
}
