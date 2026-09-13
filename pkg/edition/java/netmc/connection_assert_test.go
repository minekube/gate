package netmc

import (
	"net"
	"testing"
)

// probeConn is the interface Assert must be able to find in the tests below.
type probeConn interface{ Probe() string }

// probeConnImpl is an accepted connection that implements probeConn.
type probeConnImpl struct {
	net.Conn
}

func (c *probeConnImpl) Probe() string { return "probe" }

// maskingWrapper only embeds net.Conn (like the telemetry byte counter and the
// ConnectionEvent close tracker), which promotes net.Conn's methods and hides
// every other interface the wrapped connection implements.
type maskingWrapper struct {
	net.Conn
}

func (c *maskingWrapper) Unwrap() net.Conn { return c.Conn }

// cycleWrapper unwraps to itself, so Assert must stop instead of spinning.
type cycleWrapper struct {
	net.Conn
}

func (c *cycleWrapper) Unwrap() net.Conn { return c }

func TestAssertDescendsThroughUnwrap(t *testing.T) {
	want := &probeConnImpl{}
	got, ok := Assert[probeConn](&maskingWrapper{Conn: &maskingWrapper{Conn: want}})
	if !ok {
		t.Fatal("Assert did not descend through Unwrap; a wrapper that only embeds net.Conn masks the interfaces of the connection it wraps")
	}
	if got.Probe() != "probe" {
		t.Fatalf("Probe() = %q", got.Probe())
	}
}

func TestAssertStopsUnwrappingCycle(t *testing.T) {
	if _, ok := Assert[probeConn](&cycleWrapper{}); ok {
		t.Fatal("Assert reported a connection it cannot reach")
	}
	if _, ok := Assert[probeConn](&maskingWrapper{}); ok {
		t.Fatal("Assert reported an interface the underlying connection does not implement")
	}
}
