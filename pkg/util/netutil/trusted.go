package netutil

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// TrustedNetworks is a set of networks an address can be tested against,
// e.g. the upstreams allowed to send a PROXY protocol header.
//
// The zero value trusts nothing.
type TrustedNetworks []netip.Prefix

// ParseTrustedNetworks parses a list of IP addresses ("10.1.2.3", "::1") and
// CIDR blocks ("10.0.0.0/8", "fc00::/7") into TrustedNetworks.
func ParseTrustedNetworks(networks []string) (TrustedNetworks, error) {
	trusted := make(TrustedNetworks, 0, len(networks))
	for _, network := range networks {
		prefix, err := parseNetwork(strings.TrimSpace(network))
		if err != nil {
			return nil, err
		}
		trusted = append(trusted, prefix)
	}
	return trusted, nil
}

func parseNetwork(network string) (netip.Prefix, error) {
	if strings.Contains(network, "/") {
		prefix, err := netip.ParsePrefix(network)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("%q is not a valid IP network: %w", network, err)
		}
		if prefix.Addr().Is4In6() {
			return netip.Prefix{}, fmt.Errorf("%q is not a valid IP network: IPv4-mapped IPv6 CIDRs are not supported; use plain IPv4 CIDR form instead (e.g. %q)", network, "10.0.0.0/8")
		}
		return netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()).Masked(), nil
	}
	addr, err := netip.ParseAddr(network)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%q is not a valid IP address or network: %w", network, err)
	}
	if addr.Is4In6() {
		return netip.Prefix{}, fmt.Errorf("%q is not a valid IP address or network: IPv4-mapped IPv6 addresses are not supported; use the plain IPv4 form instead (e.g. %q)", network, "10.0.0.1")
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// Contains reports whether addr is within any of the trusted networks.
// It returns false for addresses that carry no parsable IP, so unknown
// address types are never trusted.
func (t TrustedNetworks) Contains(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	return t.ContainsStr(Host(addr))
}

// ContainsStr reports whether the host (an IP address without port)
// is within any of the trusted networks.
func (t TrustedNetworks) ContainsStr(host string) bool {
	ip, err := netip.ParseAddr(host)
	if err != nil {
		// Not an IP address (e.g. a unix socket or an in-memory net.Pipe).
		return false
	}
	ip = ip.Unmap().WithZone("")
	for _, prefix := range t {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// String returns the trusted networks in CIDR notation.
func (t TrustedNetworks) String() string {
	networks := make([]string, len(t))
	for i, prefix := range t {
		networks[i] = prefix.String()
	}
	return strings.Join(networks, ",")
}
