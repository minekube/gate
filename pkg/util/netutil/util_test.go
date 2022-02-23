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
