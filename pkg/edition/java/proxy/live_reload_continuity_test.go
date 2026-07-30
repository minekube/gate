package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/util/configutil"
)

func TestApplyLiveConfigKeepsExistingLiteSessionAndRoutesNewSession(t *testing.T) {
	firstBackend := newEchoBackend(t)
	secondBackend := newEchoBackend(t)

	const host = "play.example.test"
	cfg := config.DefaultConfig
	cfg.Bind = reserveTCPAddress(t)
	cfg.Quota.Connections.Enabled = false
	cfg.PacketLimiter.PacketsPerSecond = -1
	cfg.PacketLimiter.BytesPerSecond = -1
	cfg.Lite = liteconfig.Config{
		Enabled: true,
		Routes: []liteconfig.Route{{
			Host:         []string{host},
			Backend:      []string{firstBackend.Addr()},
			CachePingTTL: configutil.Duration(30 * time.Second),
		}},
	}

	events := event.New(event.WithRecoverPanic(false))
	ready := make(chan struct{})
	unsubscribeReady := event.Subscribe(events, 0, func(*ReadyEvent) { close(ready) })
	t.Cleanup(unsubscribeReady)

	p, err := New(Options{Config: &cfg, EventMgr: events})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	startResult := make(chan error, 1)
	go func() { startResult <- p.Start(ctx) }()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("Gate Lite did not become ready")
	}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-startResult:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("Gate Lite did not stop")
		}
	})

	existing := openLitePipe(t, cfg.Bind, host)
	defer existing.Close()
	firstBackend.WaitAccepted(t)
	requireEcho(t, existing, "before-reload")

	candidate := cfg
	candidate.Lite.Routes = append([]liteconfig.Route(nil), cfg.Lite.Routes...)
	candidate.Lite.Routes[0].Backend = []string{secondBackend.Addr()}
	require.NoError(t, p.ApplyLiveConfig(&candidate))

	requireEcho(t, existing, "after-reload")

	proposal := openLitePipe(t, cfg.Bind, host)
	defer proposal.Close()
	secondBackend.WaitAccepted(t)
	requireEcho(t, proposal, "new-session")
}

type echoBackend struct {
	ln       net.Listener
	accepted chan struct{}
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
}

func newEchoBackend(t *testing.T) *echoBackend {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	backend := &echoBackend{
		ln:       ln,
		accepted: make(chan struct{}, 1),
		conns:    make(map[net.Conn]struct{}),
	}
	backend.wg.Add(1)
	go backend.serve()
	t.Cleanup(func() {
		require.NoError(t, backend.Close())
	})
	return backend
}

func (b *echoBackend) Addr() string { return b.ln.Addr().String() }

func (b *echoBackend) WaitAccepted(t *testing.T) {
	t.Helper()
	select {
	case <-b.accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("backend did not receive Lite connection")
	}
}

func (b *echoBackend) Close() error {
	err := b.ln.Close()
	b.mu.Lock()
	for conn := range b.conns {
		_ = conn.Close()
	}
	b.mu.Unlock()
	b.wg.Wait()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

func (b *echoBackend) serve() {
	defer b.wg.Done()
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		b.conns[conn] = struct{}{}
		b.mu.Unlock()
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			defer func() {
				b.mu.Lock()
				delete(b.conns, conn)
				b.mu.Unlock()
				_ = conn.Close()
			}()
			if _, _, err := readStatusFrame(conn); err != nil {
				return
			}
			select {
			case b.accepted <- struct{}{}:
			default:
			}
			_, _ = io.Copy(conn, conn)
		}()
	}
}

func openLitePipe(t *testing.T, gateAddr, host string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", gateAddr, 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	_, portText, err := net.SplitHostPort(gateAddr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	var handshake bytes.Buffer
	pw := util.PanicWriter(&handshake)
	pw.VarInt(0)
	pw.VarInt(765)
	pw.String(host)
	require.NoError(t, util.WriteUint16(&handshake, uint16(port)))
	pw.VarInt(2)
	require.NoError(t, writeStatusFrame(conn, handshake.Bytes()))
	return conn
}

func requireEcho(t *testing.T, conn net.Conn, payload string) {
	t.Helper()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err := conn.Write([]byte(payload))
	require.NoError(t, err)
	echoed := make([]byte, len(payload))
	_, err = io.ReadFull(conn, echoed)
	require.NoError(t, err)
	require.Equal(t, payload, string(echoed), fmt.Sprintf("echo mismatch for %q", payload))
}
