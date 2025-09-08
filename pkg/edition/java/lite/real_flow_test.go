package lite

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
)

// TestDialRouteConnectionRefusedVerbosity tests the actual dialRoute function
// to verify that connection refused errors get the right verbosity
func TestDialRouteConnectionRefusedVerbosity(t *testing.T) {
	// Create a mock context
	ctx := context.Background()
	dialTimeout := time.Second * 5
	srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
	route := &config.Route{}

	// Try to dial a port that should be refused (port 1 is usually closed)
	backendAddr := "127.0.0.1:1"
	handshake := &packet.Handshake{}
	handshakeCtx := &proto.PacketContext{}

	// This should fail with connection refused
	_, err := dialRoute(ctx, dialTimeout, srcAddr, route, backendAddr, handshake, handshakeCtx, false)

	assert.Error(t, err, "Should fail to connect to closed port")

	// Check if it's a VerbosityError
	var verbErr *errs.VerbosityError
	if assert.True(t, errors.As(err, &verbErr), "Should be a VerbosityError") {
		t.Logf("Error: %v", err)
		t.Logf("Verbosity: %d", verbErr.Verbosity)

		// Check if our IsConnectionRefused detects it
		isRefused := IsConnectionRefused(err)
		t.Logf("IsConnectionRefused: %v", isRefused)

		if isRefused {
			assert.Equal(t, 1, verbErr.Verbosity, "Connection refused should have verbosity 1 (debug level)")
		}
	}
}

// TestTryBackendsRealFlow tests tryBackends with actual dialRoute calls
func TestTryBackendsRealFlow(t *testing.T) {
	log := testr.New(t)

	// Create nextBackend that returns non-existent backends
	backends := []string{"127.0.0.1:1", "127.0.0.1:2"} // These ports should be closed
	remainingBackends := make([]string, len(backends))
	copy(remainingBackends, backends)

	nextBackend := func() (string, logr.Logger, bool) {
		if len(remainingBackends) == 0 {
			return "", log, false
		}
		backend := remainingBackends[0]
		remainingBackends = remainingBackends[1:]
		return backend, log.WithValues("backendAddr", backend), true
	}

	// Try function that calls actual dialRoute
	tryFunc := func(log logr.Logger, backendAddr string) (logr.Logger, net.Conn, error) {
		ctx := context.Background()
		dialTimeout := time.Second * 1
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
		route := &config.Route{}
		handshake := &packet.Handshake{}
		handshakeCtx := &proto.PacketContext{}

		conn, err := dialRoute(ctx, dialTimeout, srcAddr, route, backendAddr, handshake, handshakeCtx, false)
		return log, conn, err
	}

	// This should try both backends and both should fail
	_, _, _, err := tryBackends(nextBackend, tryFunc)

	assert.Equal(t, errAllBackendsFailed, err, "Should fail when all backends are unreachable")

	t.Log("Check the test output to see what verbosity level the 'failed to try backend' logs use")
}
