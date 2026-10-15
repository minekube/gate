package lite

import (
	"net"
	"net/netip"
	"slices"
	"strconv"

	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/lite/voicechat"
)

// VoiceListens returns the distinct, sorted UDP listen addresses of all routes with voiceChat enabled.
func VoiceListens(routes []config.Route) []string {
	var out []string
	for i := range routes {
		if v := routes[i].VoiceChat; v != nil {
			out = append(out, v.ListenAddr())
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// registerVoice records the player's TCP session so the voice relay may forward
// that client IP's UDP traffic to the selected backend. It never returns nil.
func registerVoice(voice *voicechat.Manager, route *config.Route, backendAddr string, client net.Addr) func() {
	noop := func() {}
	if voice == nil || route == nil || route.VoiceChat == nil {
		return noop
	}
	ip, ok := clientIP(client)
	if !ok {
		return noop
	}
	return voice.Register(ip, route.VoiceChat.ListenAddr(), voiceBackendAddr(backendAddr, route.VoiceChat.BackendUDPPort()))
}

// voiceBackendAddr swaps the TCP port of a backend address for the voice UDP port.
func voiceBackendAddr(backendAddr string, port int) string {
	host, _, err := net.SplitHostPort(backendAddr)
	if err != nil {
		host = backendAddr // no port given
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func clientIP(addr net.Addr) (netip.Addr, bool) {
	var ip net.IP
	switch a := addr.(type) {
	case *net.TCPAddr:
		ip = a.IP
	case *net.UDPAddr:
		ip = a.IP
	default:
		return netip.Addr{}, false
	}
	nip, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return nip.Unmap(), true
}
