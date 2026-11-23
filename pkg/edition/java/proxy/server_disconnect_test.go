package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// A dial can succeed after player teardown has already tried to close the
// in-flight connection. That late socket must be closed, never published or
// sent a handshake. Force the ordering in the dialer instead of racing sleeps.
func TestServerConnectionClosesDialResultAfterTeardown(t *testing.T) {
	proxySide, backendSide := net.Pipe()
	t.Cleanup(func() { _ = proxySide.Close(); _ = backendSide.Close() })
	if err := backendSide.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}

	p := newDuplicateLoginTestProxy()
	cfg := config.DefaultConfig
	cfg.Forwarding.Mode = config.NoneForwardingMode
	p.cfg = &cfg
	clientConn := newDuplicateLoginTestConn(state.Play)
	t.Cleanup(func() { _ = clientConn.Close() })
	player := newDuplicateLoginTestPlayer(p, clientConn, "ForgeCancel", uuid.New())
	player.virtualHost = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 25565}
	if !p.registerConnection(player) {
		t.Fatal("could not register player")
	}
	s := newServerConnection(newRegisteredServer(&dynamicDialServer{
		name: "forge",
		addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 25565},
		dial: func(context.Context, Player) (net.Conn, error) {
			player.teardown()
			return proxySide, nil
		},
	}), nil, player)
	player.setInFlightConnection(s)

	done := make(chan struct{})
	var connectErr error
	go func() {
		_, connectErr = s.connect(context.Background())
		close(done)
	}()
	t.Cleanup(func() {
		_ = backendSide.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("connect did not return after closing the backend")
		}
	})

	var buf [1]byte
	if n, err := backendSide.Read(buf[:]); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("backend received (%d bytes, %v), want EOF without a handshake after teardown", n, err)
	}
	select {
	case <-done:
		if !errors.Is(connectErr, netmc.ErrClosedConn) {
			t.Fatalf("connect error = %v, want ErrClosedConn", connectErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connect did not return; socket close may be holding the server lock")
	}
	if _, connected := s.ensureConnected(); connected {
		t.Error("late backend socket was published after teardown")
	}
	if p.PlayerCount() != 0 {
		t.Error("player registration survived teardown")
	}
}

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
