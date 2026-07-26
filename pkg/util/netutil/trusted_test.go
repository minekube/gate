package netutil

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTrustedNetworks(t *testing.T) {
	trusted, err := ParseTrustedNetworks([]string{
		"127.0.0.0/8",
		"10.1.2.3",
		"fc00::/7",
		" 2001:db8::1 ",
	})
	require.NoError(t, err)
	require.Equal(t, "127.0.0.0/8,10.1.2.3/32,fc00::/7,2001:db8::1/128", trusted.String())

	for _, invalid := range []string{
		"",
		"not-an-ip",
		"10.0.0.0/33",
		"10.0.0.0/",
		"10.0.0.256",
		"::ffff:10.0.0.0/104",
		"::ffff:10.0.0.1",
	} {
		_, err := ParseTrustedNetworks([]string{invalid})
		require.Errorf(t, err, "expected %q to be rejected", invalid)
	}
}

func TestTrustedNetworksContains(t *testing.T) {
	trusted, err := ParseTrustedNetworks([]string{"127.0.0.0/8", "10.0.0.0/8", "fc00::/7", "192.0.2.7"})
	require.NoError(t, err)

	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:25565", true},
		{"10.9.8.7:1234", true},
		{"[fdaa:0:1::3]:25565", true}, // Fly.io 6PN
		{"[::ffff:10.0.0.1]:25565", true},
		{"192.0.2.7:80", true},
		{"192.0.2.8:80", false},
		{"1.2.3.4:25565", false},
		{"[2001:db8::1]:25565", false},
		{"[fe80::1%eth0]:25565", false},
		{"127.0.0.1", true}, // no port
	}
	for _, tt := range tests {
		got := trusted.Contains(NewAddr(tt.addr, "tcp"))
		require.Equalf(t, tt.want, got, "Contains(%q)", tt.addr)
	}

	// netip.Prefix carries no zone, so a zoned address must be matched by its IP.
	linkLocal, err := ParseTrustedNetworks([]string{"fe80::/10"})
	require.NoError(t, err)
	require.True(t, linkLocal.Contains(NewAddr("[fe80::1%eth0]:25565", "tcp")))

	require.False(t, trusted.Contains(nil))
	require.False(t, trusted.Contains(NewAddr("/tmp/gate.sock", "unix")), "non-IP addresses are never trusted")

	var none TrustedNetworks
	require.False(t, none.Contains(NewAddr("127.0.0.1:25565", "tcp")), "the zero value trusts nothing")

	var _ net.Addr = NewAddr("127.0.0.1:1", "tcp")
}
