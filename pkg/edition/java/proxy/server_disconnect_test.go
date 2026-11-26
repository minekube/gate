package proxy

import (
	"context"
	"net"
	"testing"
	"time"

	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
)

// serverStateReadingHandler mimics what every backend session handler does in
// Disconnected(): it reads serverConnection state while netmc is still inside
// Close(). The real handlers call reportStalledBackend (backendStallSnapshot ->
// serverConnection.mu) and/or ensureConnected/active, all of which take the same
// mutex.
type serverStateReadingHandler struct {
	nopSessionHandler
	conn         *serverConnection
	disconnected chan struct{}
}

func (h *serverStateReadingHandler) Disconnected() {
	_ = h.conn.conn() // serverConnection.mu.RLock, exactly like backendStallSnapshot
	close(h.disconnected)
}

// A backend connection is closed from inside its own session handler (a backend
// Disconnect packet in CONFIGURATION, a login failure, ...). netmc calls the
// active handler's Disconnected() synchronously from Close(), so closing the
// connection while holding serverConnection.mu deadlocks the teardown on a
// non-reentrant mutex: the handler goroutine parks forever holding the lock, the
// connection stays "connected", and a later player teardown that reaches for the
// same serverConnection blocks behind it - which is how a config-phase backend
// kick left the player registered and the next join refused with
// "You are already connected to this server!".
func TestServerConnectionDisconnectReleasesLockBeforeHandlerTeardown(t *testing.T) {
	proxySide, backendSide := net.Pipe()
	defer backendSide.Close()

	mc, _ := netmc.NewMinecraftConn(context.Background(), proxySide, proto.ClientBound, 0, 0, 0, nil)

	h := &serverStateReadingHandler{disconnected: make(chan struct{})}
	s := &serverConnection{connection: mc}
	h.conn = s
	mc.SetActiveSessionHandler(state.Config, h)

	done := make(chan struct{})
	go func() {
		s.disconnect()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("serverConnection.disconnect() did not return within 5s: it holds its mutex across the " +
			"connection Close(), while the session handler's Disconnected() reads that same mutex - a " +
			"self-deadlock that parks the backend teardown forever")
	}
	select {
	case <-h.disconnected:
	case <-time.After(5 * time.Second):
		t.Fatal("the active session handler's Disconnected() was never called while closing the connection")
	}
}
