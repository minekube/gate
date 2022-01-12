package netutil

import (
	"net"
	"strconv"
)

// HostPortAddr provides the host and port of an address in cases where
// use of host, port, err := net.SplitHostPort(addressString) is too much
// and an error is unexpected or ignored.
type HostPortAddr interface {
	// Host returns the host part from the address.
	Host() string
	// Port returns the port part from the address.
	// Zero value means the port is unspecified.
	Port() uint16
}

// Host is like HostPort, but directly returns the result of HostPortAddr.Host().
func Host(addr net.Addr) string {
	hp, _ := WrapAddr(addr)
	return hp.(HostPortAddr).Host()
}

// Port is like HostPort, but directly returns the result of HostPortAddr.Port().
func Port(addr net.Addr) uint16 {
	hp, _ := WrapAddr(addr)
	return hp.(HostPortAddr).Port()
}

// HostPort is like Addr, but directly returns it's HostPortAddr.
func HostPort(addr net.Addr) HostPortAddr {
	hp, _ := WrapAddr(addr)
	return hp.(HostPortAddr)
}

// Addr prepares a net.Addr to be used with HostPort and ignores any error.
func Addr(addr net.Addr) net.Addr {
	a, _ := WrapAddr(addr)
	return a
}

// WrapAddr prepares a net.Addr to be used with HostPort.
func WrapAddr(addr net.Addr) (net.Addr, error) {
	if _, ok := addr.(HostPortAddr); ok {
		return addr, nil
	}
	var (
		port string
		err        error
		p          int
		a          = &address{Addr: addr}
	)
	a.host, port, err = net.SplitHostPort(addr.String())
	if err == nil {
		p, err = strconv.Atoi(port)
	}
	a.port = uint16(p)
	return a, err
}

// WrapConn adds HostPortAddr to RemoteAddr and LocalAddr of the passed net.Conn
// such that using HostPort(conn.RemoteAddr()) returns the prepared HostPortAddr.
// Wrapping a net.Conn is useful when it is expected that HostPort is used often.
func WrapConn(conn net.Conn) (net.Conn, error) {
	remoteAddr, err := WrapAddr(conn.RemoteAddr())
	if err != nil {
		return nil, err
	}
	localAddr, err := WrapAddr(conn.LocalAddr())
	if err != nil {
		return nil, err
	}
	return &connection{
		Conn:       conn,
		remoteAddr: remoteAddr,
		localAddr:  localAddr,
	}, nil
}

// Parse parses addr and constructs a net.Addr compatible with HostPort.
func Parse(addr string, network string) (net.Addr, error) {
	host, port, err := net.SplitHostPort(addr)
	var p int
	if err == nil {
		p, err = strconv.Atoi(port)
	}
	return &address{
		Addr: &customAddr{
			network: network,
			str:     addr,
		},
		host: host,
		port: uint16(p),
	}, err
}

// NewAddr returns a new net.Addr ready to use with HostPort.
func NewAddr(network, host string, port uint16) net.Addr {
	return &address{
		Addr: &customAddr{
			network: network,
			str:     net.JoinHostPort(host, strconv.Itoa(int(port))),
		},
		host: host,
		port: port,
	}
}

type customAddr struct{ network, str string }

func (c *customAddr) Network() string { return c.network }
func (c *customAddr) String() string  { return c.str }

var _ net.Addr = (*customAddr)(nil)

type (
	connection struct {
		net.Conn
		remoteAddr net.Addr
		localAddr  net.Addr
	}
	address struct {
		net.Addr
		host string
		port uint16
	}
)

func (s *address) Host() string { return s.host }
func (s *address) Port() uint16 { return s.port }

func (s *connection) RemoteAddr() net.Addr { return s.remoteAddr }
func (s *connection) LocalAddr() net.Addr  { return s.localAddr }
