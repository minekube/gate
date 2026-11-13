package netutil

import (
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
)

// Host returns the host of net.Addr.
//
// Like HostStr it has no error return: the host part is still usable when only
// the port is malformed. Callers that must reject an address use Parse.
func Host(addr net.Addr) string {
	return HostStr(addr.String())
}

// HostStr returns the host of the address.
//
// The parse error of the port is deliberately discarded here (this accessor
// has no error return and is used on hot logging, quota and dial paths): the
// host part is returned whenever the address carries a host, even if its port
// is malformed. Callers that must reject a malformed address use Parse.
func HostStr(addr string) string {
	host, _, _ := splitHostPort(addr)
	return host
}

// Port returns the port of net.Addr, or 0 when the address has no port or its
// port is unusable (non-numeric or outside 0-65535). 0 is the package's "no
// port" sentinel and is what Parse strips; an unusable port is never reported
// as a wrapped value. Callers that must reject the address use Parse.
func Port(addr net.Addr) uint16 {
	_, port, _ := splitHostPort(addr.String())
	return port
}

// PortStr returns the port of the address, or 0 when the address has no port
// or its port is unusable (see Port for the fallback contract).
func PortStr(addr string) uint16 {
	_, port, _ := splitHostPort(addr)
	return port
}

// HostPort returns the split host and port of a net.Addr.
//
// An address without a port, or with an unusable port (non-numeric or outside
// 0-65535), yields port 0 - the package's "no port" sentinel - never a wrapped
// port (see Port). Callers that must reject the address use Parse.
func HostPort(addr net.Addr) (host string, port uint16) {
	host, port, _ = splitHostPort(addr.String())
	return
}

// Parse parses addr and constructs a net.Addr with
// the specified network. A 0 port is removed.
//
// An address whose port is malformed (non-numeric or outside 0-65535) is
// rejected with an error instead of being silently narrowed into a different
// port; the address is then returned unchanged next to the error.
func Parse(addr string, network string) (net.Addr, error) {
	host, port, err := splitHostPort(addr)
	if err != nil {
		// Keep the address verbatim: only a successfully parsed address may
		// be rewritten (a rejected one must not lose its port).
		return &address{addr: addr, network: network}, err
	}
	if port == 0 {
		addr = host
	}
	return &address{addr: addr, network: network}, nil
}

// NewAddr creates a new net.Addr without format validation.
func NewAddr(addr, network string) net.Addr {
	return &address{addr: addr, network: network}
}

// splitHostPort splits an address into host and port. An address without a
// port is not an error: the whole address is returned as host with port 0.
//
// A port that does not fit into the uint16 it is returned as is an error
// (port 0). strconv.Atoi does not range check its result, so narrowing it
// directly would silently wrap it (70000 -> 4464, -1 -> 65535); the explicit
// bounds check below rejects such a port instead of reporting a different one.
func splitHostPort(addr string) (host string, port uint16, err error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		if isMissingPortErr(err) || isTooManyColonsErr(err) {
			// Not an error: this address simply carries no port.
			return addr, 0, nil
		}
		return host, 0, err
	}
	portInt, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 0, fmt.Errorf("invalid port %q in address %q: %w", portStr, addr, err)
	}
	if portInt < 0 || portInt > math.MaxUint16 {
		return host, 0, fmt.Errorf("port %s in address %q out of range (0-%d)", portStr, addr, math.MaxUint16)
	}
	return host, uint16(portInt), nil
}

type address struct{ network, addr string }

func (c *address) Network() string { return c.network }
func (c *address) String() string  { return c.addr }

var _ net.Addr = (*address)(nil)

func isMissingPortErr(err error) bool {
	var addrErr *net.AddrError
	return err != nil && errors.As(err, &addrErr) && addrErr.Err == "missing port in address"
}

func isTooManyColonsErr(err error) bool {
	var addrErr *net.AddrError
	return err != nil && errors.As(err, &addrErr) && addrErr.Err == "too many colons in address"
}
