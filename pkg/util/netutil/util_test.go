package netutil

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHost(t *testing.T) {
	host := Host(&address{addr: "host:123"})
	require.Equal(t, "host", host)
}

func TestPort(t *testing.T) {
	v := Port(&address{addr: "host:123"})
	require.Equal(t, uint16(123), v)
}

func TestParse(t *testing.T) {
	addr, err := Parse("host:123", "some network")
	require.NoError(t, err)
	require.Equal(t, "host:123", addr.String())
	require.Equal(t, "some network", addr.Network())
}

func TestSplitHostPort_isMissingPortErr(t *testing.T) {
	_, _, err := net.SplitHostPort("host-without-port")
	require.True(t, isMissingPortErr(err))
}

func TestHostPort_BareIPv6(t *testing.T) {
	// Bare IPv6 addresses without brackets and port (e.g. from connect.Addr)
	// should return the full address as host with port 0.
	tests := []struct {
		addr string
		host string
		port uint16
	}{
		{"2a09:bac6:d73f:28::4:31d", "2a09:bac6:d73f:28::4:31d", 0},
		{"2a09:bac6:d73f:3046::4cf:74", "2a09:bac6:d73f:3046::4cf:74", 0},
		{"::1", "::1", 0},
		{"[::1]:25565", "::1", 25565},
		{"127.0.0.1:25565", "127.0.0.1", 25565},
	}
	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			addr := &address{addr: tc.addr}
			host, port := HostPort(addr)
			require.Equal(t, tc.host, host)
			require.Equal(t, tc.port, port)
		})
	}
}
