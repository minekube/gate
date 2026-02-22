package lite

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
)

func TestEffectiveClientIPFromRemoteAddr(t *testing.T) {
	ip := effectiveClientIP(nil, "play.example.com", &net.TCPAddr{
		IP:   net.ParseIP("203.0.113.1"),
		Port: 25565,
	})
	assert.Equal(t, netip.MustParseAddr("203.0.113.1"), ip)
}

func TestEffectiveClientIPPrefersTCPShieldMetadata(t *testing.T) {
	route := &liteconfig.Route{TCPShieldRealIP: true}
	ip := effectiveClientIP(route, "play.example.com///198.51.100.5:41234///1700000000", &net.TCPAddr{
		IP:   net.ParseIP("203.0.113.2"),
		Port: 25565,
	})
	assert.Equal(t, netip.MustParseAddr("198.51.100.5"), ip)
}

func TestEffectiveClientIPFallsBackOnInvalidTCPShieldMetadata(t *testing.T) {
	route := &liteconfig.Route{TCPShieldRealIP: true}
	ip := effectiveClientIP(route, "play.example.com///not-an-ip///1700000000", &net.TCPAddr{
		IP:   net.ParseIP("203.0.113.3"),
		Port: 25565,
	})
	assert.Equal(t, netip.MustParseAddr("203.0.113.3"), ip)
}
