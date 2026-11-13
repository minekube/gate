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

// TestSplitHostPort_PortRange is regression coverage for CodeQL alert #1
// ("Incorrect conversion between integer types"): splitHostPort narrowed an
// unbounded strconv.Atoi result into a uint16, so "host:70000" silently became
// port 4464 and "host:-1" became 65535. An out-of-range port must be rejected
// (the returned port stays 0, the package's "no port" sentinel) and must never
// be wrapped into a different port.
func TestSplitHostPort_PortRange(t *testing.T) {
	tests := []struct {
		addr    string
		host    string
		port    uint16
		wantErr bool
	}{
		// In range: parsed as-is, including the values closest to the bounds.
		{addr: "host:0", host: "host", port: 0},
		{addr: "host:1", host: "host", port: 1},
		{addr: "host:25565", host: "host", port: 25565},
		{addr: "host:32768", host: "host", port: 32768},
		{addr: "host:65535", host: "host", port: 65535},
		{addr: "[::1]:65535", host: "::1", port: 65535},
		// Out of range: rejected, port is never a wrapped value.
		{addr: "host:65536", host: "host", wantErr: true},
		{addr: "host:70000", host: "host", wantErr: true},
		{addr: "host:131072", host: "host", wantErr: true},
		{addr: "host:-1", host: "host", wantErr: true},
		{addr: "host:-65535", host: "host", wantErr: true},
		{addr: "host:99999999999999999999", host: "host", wantErr: true},
		// Malformed: rejected.
		{addr: "host:abc", host: "host", wantErr: true},
		{addr: "host:1.5", host: "host", wantErr: true},
		{addr: "host:", host: "host", wantErr: true},
		// No port at all: the whole address is the host (missing-port path).
		{addr: "host", host: "host"},
		{addr: "2a09:bac6:d73f:28::4:31d", host: "2a09:bac6:d73f:28::4:31d"},
		{addr: "::1", host: "::1"},
	}
	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			host, port, err := splitHostPort(tc.addr)
			require.Equal(t, tc.host, host)
			if tc.wantErr {
				require.Error(t, err,
					"out-of-range/malformed port must be rejected, got port %d", port)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.port, port, "port must never be a wrapped value")
		})
	}
}

// TestParse_RejectsOutOfRangePort pins the validating entry point: config and
// proxy-protocol addresses go through Parse, which propagates the error, so a
// wrapped port can no longer be accepted silently. A rejected address is
// returned unchanged (no port stripping) next to its error.
func TestParse_RejectsOutOfRangePort(t *testing.T) {
	for _, addr := range []string{"host:65536", "host:70000", "host:-1"} {
		t.Run(addr, func(t *testing.T) {
			parsed, err := Parse(addr, "tcp")
			require.Error(t, err)
			require.ErrorContains(t, err, "out of range")
			require.NotNil(t, parsed)
			require.Equal(t, addr, parsed.String())
		})
	}

	// Valid addresses keep the documented behavior: a 0 port is stripped.
	parsed, err := Parse("host:25565", "tcp")
	require.NoError(t, err)
	require.Equal(t, "host:25565", parsed.String())

	parsed, err = Parse("host:0", "tcp")
	require.NoError(t, err)
	require.Equal(t, "host", parsed.String())
}

// TestPortAccessors_OutOfRangePort documents the deliberate fallback of the
// error-discarding accessors (no error return; used on logging, quota and
// backend-dial paths): an unusable port reads as 0 - the package's "no port"
// sentinel - and never as a wrapped port. Callers that must reject an address
// use Parse instead.
func TestPortAccessors_OutOfRangePort(t *testing.T) {
	for _, addr := range []string{"host:65536", "host:70000", "host:-1", "host:99999999999999999999"} {
		t.Run(addr, func(t *testing.T) {
			require.Equal(t, uint16(0), PortStr(addr))
			require.Equal(t, uint16(0), Port(&address{addr: addr}))

			_, port := HostPort(&address{addr: addr})
			require.Equal(t, uint16(0), port)

			// The host part is unambiguous, so it is still returned.
			require.Equal(t, "host", HostStr(addr))
			require.Equal(t, "host", Host(&address{addr: addr}))
		})
	}
}
