package protoutil

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/connect"
)

func TestProxyHeader(t *testing.T) {
	srcAddr := connect.Addr("104.28.243.188:0")
	destAddr := connect.Addr("127.0.0.1:25565")

	v4Addr := &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 25565,
	}
	v6Addr := &net.TCPAddr{
		IP:   net.IPv6loopback,
		Port: 25566,
	}

	// Bare IPv6 addresses without brackets and port (as reported in issue #670)
	bareIPv6Src := connect.Addr("2a09:bac6:d73f:28::4:31d")
	bareIPv6Src2 := connect.Addr("2a09:bac6:d73f:3046::4cf:74")

	testCases := []struct {
		name           string
		srcAddr        net.Addr
		destAddr       net.Addr
		skipStrCompare bool // bare IPv6 addr strings differ from net.TCPAddr strings
	}{
		{"virtual addr should not fail to write proxy header", srcAddr, destAddr, false},
		{"mix of v4 and v6 should not fail to write proxy header", v4Addr, v6Addr, false},
		{"mix of v6 and v4 should not fail to write proxy header", v6Addr, v4Addr, false},
		{"v4 addr should not fail to write proxy header", v4Addr, v4Addr, false},
		{"v6 addr should not fail to write proxy header", v6Addr, v6Addr, false},
		{"mix of v4 and virtual should not fail to write proxy header", v4Addr, destAddr, false},
		{"mix of v6 and virtual should not fail to write proxy header", v6Addr, destAddr, false},
		{"mix of virtual and v4 should not fail to write proxy header", srcAddr, v4Addr, false},
		{"mix of virtual and v6 should not fail to write proxy header", srcAddr, v6Addr, false},
		{"bare IPv6 src should not panic", bareIPv6Src, destAddr, true},
		{"bare IPv6 src2 should not panic", bareIPv6Src2, destAddr, true},
		{"bare IPv6 src with v4 dest should not panic", bareIPv6Src, v4Addr, true},
		{"bare IPv6 src with v6 dest should not panic", bareIPv6Src, v6Addr, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			header := ProxyHeader(tc.srcAddr, tc.destAddr)
			require.NotNil(t, header)
			require.NotNil(t, header.SourceAddr)
			require.NotNil(t, header.DestinationAddr)
			if !tc.skipStrCompare {
				require.Equal(t, tc.srcAddr.String(), header.SourceAddr.String())
				require.Equal(t, tc.destAddr.String(), header.DestinationAddr.String())
			}

			// Create a buffer to write the header
			buf := new(bytes.Buffer)
			_, err := header.WriteTo(buf)
			require.NoError(t, err)
		})
	}
}
