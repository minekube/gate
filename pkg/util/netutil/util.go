package netutil

import (
	"errors"
	"net"
	"strconv"
)

// Host returns the host of net.Addr.
func Host(addr net.Addr) string {
	return HostStr(addr.String())
}

// HostStr returns the host of the address.
func HostStr(addr string) string {
	host, _, _ := splitHostPort(addr)
	return host
}

// Port returns the port of net.Addr.
func Port(addr net.Addr) uint16 {
	_, port, _ := splitHostPort(addr.String())
	return port
}

// PortStr returns the port of the address.
func PortStr(addr string) uint16 {
	_, port, _ := splitHostPort(addr)
	return port
}

// HostPort returns the split host and port of a net.Addr.
func HostPort(addr net.Addr) (host string, port uint16) {
	host, port, _ = splitHostPort(addr.String())
	return
}

// Parse parses addr and constructs a net.Addr with
// the specified network. A 0 port is removed.
func Parse(addr string, network string) (net.Addr, error) {
	host, port, err := splitHostPort(addr)
	if port == 0 {
		addr = host
	}
	return &address{addr: addr, network: network}, err
}

// NewAddr creates a new net.Addr without format validation.
func NewAddr(addr, network string) net.Addr {
	return &address{addr: addr, network: network}
}

func splitHostPort(addr string) (host string, port uint16, err error) {
	portInt := 0
	portStr := ""
	host, portStr, err = net.SplitHostPort(addr)
	if err == nil {
		portInt, err = strconv.Atoi(portStr)
	} else if isMissingPortErr(err) {
		host = addr
		err = nil
	}
	return host, uint16(portInt), err
}

type address struct{ network, addr string }

func (c *address) Network() string { return c.network }
func (c *address) String() string  { return c.addr }

var _ net.Addr = (*address)(nil)

func isMissingPortErr(err error) bool {
	var addrErr *net.AddrError
	return err != nil && errors.As(err, &addrErr) && addrErr.Err == "missing port in address"
}
