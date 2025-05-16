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

	testCases := []struct {
		name     string
		srcAddr  net.Addr
		destAddr net.Addr
	}{
		{"virtual addr should not fail to write proxy header", srcAddr, destAddr},
		{"mix of v4 and v6 should not fail to write proxy header", v4Addr, v6Addr},
		{"mix of v6 and v4 should not fail to write proxy header", v6Addr, v4Addr},
		{"v4 addr should not fail to write proxy header", v4Addr, v4Addr},
		{"v6 addr should not fail to write proxy header", v6Addr, v6Addr},
		{"mix of v4 and virtual should not fail to write proxy header", v4Addr, destAddr},
		{"mix of v6 and virtual should not fail to write proxy header", v6Addr, destAddr},
		{"mix of virtual and v4 should not fail to write proxy header", srcAddr, v4Addr},
		{"mix of virtual and v6 should not fail to write proxy header", srcAddr, v6Addr},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			header := ProxyHeader(tc.srcAddr, tc.destAddr)
			require.NotNil(t, header)
			require.NotNil(t, header.SourceAddr)
			require.NotNil(t, header.DestinationAddr)
			require.Equal(t, tc.srcAddr.String(), header.SourceAddr.String())
			require.Equal(t, tc.destAddr.String(), header.DestinationAddr.String())

			// Create a buffer to write the header
			buf := new(bytes.Buffer)
			_, err := header.WriteTo(buf)
			require.NoError(t, err)
		})
	}
}
