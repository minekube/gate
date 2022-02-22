package netutil

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type someInterface interface {
	implemented()
}

type someType struct {
	connection
}

func (someType) implemented() {}

func TestDontHideInterface(t *testing.T) {
	c, err := WrapConn(&someType{connection{ // TODO
		Conn: nil,
		remoteAddr: address{
			Addr: &customAddr{
				str: "remoteAddr",
			},
			host: "remoteAddr",
		},
		localAddr: address{
			Addr: &customAddr{
				str: "localAddr",
			},
			host: "localAddr",
		},
	}})
	require.NoError(t, err)
	_, ok := c.(someInterface)
	require.True(t, ok, "netutil.WrapConn should not hide interfaces that wrapped net.Conn implements")
}

func TestSplitHostPort_isMissingPortErr(t *testing.T) {
	_, _, err := net.SplitHostPort("host-without-port")
	require.True(t, isMissingPortErr(err))
}
