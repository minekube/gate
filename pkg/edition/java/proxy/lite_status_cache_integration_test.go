package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/lite"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/util/configutil"
)

func TestLiteStatusCacheWireFlow(t *testing.T) {
	lite.ResetPingCache()
	t.Cleanup(lite.ResetPingCache)

	backend := newStatusBackend(t)
	defer backend.Close()

	const host = "status.example.com"
	cfg := config.DefaultConfig
	cfg.Bind = reserveTCPAddress(t)
	cfg.Quota.Connections.Enabled = false
	cfg.PacketLimiter.PacketsPerSecond = -1
	cfg.PacketLimiter.BytesPerSecond = -1
	cfg.Lite = liteconfig.Config{
		Enabled: true,
		Routes: []liteconfig.Route{{
			Host:         []string{host},
			Backend:      []string{backend.Addr()},
			CachePingTTL: configutil.Duration(2 * time.Second),
		}},
	}

	events := event.New(event.WithRecoverPanic(false))
	ready := make(chan struct{})
	unsubscribeReady := event.Subscribe(events, 0, func(*ReadyEvent) {
		select {
		case <-ready:
		default:
			close(ready)
		}
	})
	defer unsubscribeReady()

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

	ping := func(protocol int) string {
		result, err := statusPing(cfg.Bind, host, protocol)
		require.NoError(t, err)
		return result
	}

	t.Run("protocol keys are isolated", func(t *testing.T) {
		legacy := ping(47)
		modern := ping(765)
		require.Equal(t, legacy, ping(47))
		require.Equal(t, modern, ping(765))
		require.NotEqual(t, legacy, modern)
		require.Equal(t, int32(2), backend.Fetches())
		t.Logf("protocol 47 -> %s; protocol 765 -> %s; hot repeats caused 0 backend fetches", legacy, modern)
	})

	t.Run("concurrent misses are singleflight", func(t *testing.T) {
		lite.ResetPingCache()
		backend.SetDelay(250 * time.Millisecond)
		before := backend.Fetches()

		const clients = 16
		start := make(chan struct{})
		type pingResult struct {
			status string
			err    error
		}
		results := make(chan pingResult, clients)
		var wg sync.WaitGroup
		wg.Add(clients)
		for range clients {
			go func() {
				defer wg.Done()
				<-start
				status, err := statusPing(cfg.Bind, host, 765)
				results <- pingResult{status: status, err: err}
			}()
		}
		close(start)
		wg.Wait()
		close(results)

		var shared string
		for result := range results {
			require.NoError(t, result.err)
			if shared == "" {
				shared = result.status
			}
			require.Equal(t, shared, result.status)
		}
		require.Equal(t, int32(1), backend.Fetches()-before)
		t.Logf("%d simultaneous client pings -> %s; backend fetch delta=1", clients, shared)
	})

	t.Run("hot reads do not extend ttl", func(t *testing.T) {
		lite.ResetPingCache()
		backend.SetDelay(0)
		before := backend.Fetches()
		initial := ping(765)
		deadline := time.Now().Add(4 * time.Second)
		var refreshed string
		hotReads := 0
		for time.Now().Before(deadline) {
			status := ping(765)
			if status != initial {
				refreshed = status
				break
			}
			hotReads++
			time.Sleep(25 * time.Millisecond)
		}

		require.NotEmpty(t, refreshed, "cache entry never expired while continuously read")
		require.Positive(t, hotReads, "test must observe at least one cached hot read")
		require.Equal(t, int32(2), backend.Fetches()-before)
		t.Logf("initial=%s; %d hot reads stayed cached until insertion-based expiry; refreshed=%s; backend fetch delta=2", initial, hotReads, refreshed)
	})

	t.Run("route ttl reload invalidates globally", func(t *testing.T) {
		cached := ping(765)
		before := backend.Fetches()
		previous := cfg
		next := cfg
		next.Lite.Routes = append([]liteconfig.Route(nil), cfg.Lite.Routes...)
		next.Lite.Routes[0].CachePingTTL = configutil.Duration(5 * time.Second)

		reload.FireConfigUpdate(events, &next, &previous)
		refreshed := ping(765)

		require.NotEqual(t, cached, refreshed)
		require.Equal(t, int32(1), backend.Fetches()-before)
		t.Logf("cachePingTTL reload 2s -> 5s invalidated %s; next client received %s immediately", cached, refreshed)
	})
}

type statusBackend struct {
	t       *testing.T
	ln      net.Listener
	fetches atomic.Int32
	delayNS atomic.Int64
	wg      sync.WaitGroup
}

func newStatusBackend(t *testing.T) *statusBackend {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	b := &statusBackend{t: t, ln: ln}
	b.wg.Add(1)
	go b.serve()
	return b
}

func (b *statusBackend) Addr() string { return b.ln.Addr().String() }

func (b *statusBackend) Fetches() int32 { return b.fetches.Load() }

func (b *statusBackend) SetDelay(d time.Duration) { b.delayNS.Store(int64(d)) }

func (b *statusBackend) Close() {
	_ = b.ln.Close()
	b.wg.Wait()
}

func (b *statusBackend) serve() {
	defer b.wg.Done()
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			defer conn.Close()
			if err := b.handle(conn); err != nil {
				b.t.Errorf("mock status backend: %v", err)
			}
		}()
	}
}

func (b *statusBackend) handle(conn net.Conn) error {
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	id, handshake, err := readStatusFrame(conn)
	if err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}
	if id != 0 {
		return fmt.Errorf("handshake packet id=%d, want 0", id)
	}
	protocol, err := util.ReadVarInt(bytes.NewReader(handshake))
	if err != nil {
		return fmt.Errorf("read protocol: %w", err)
	}
	id, _, err = readStatusFrame(conn)
	if err != nil {
		return fmt.Errorf("read status request: %w", err)
	}
	if id != 0 {
		return fmt.Errorf("status request packet id=%d, want 0", id)
	}

	fetch := b.fetches.Add(1)
	time.Sleep(time.Duration(b.delayNS.Load()))
	status := fmt.Sprintf(`{"version":{"name":"mock","protocol":%d},"players":{"max":20,"online":%d},"description":{"text":"backend-fetch-%d-protocol-%d"}}`, protocol, fetch, fetch, protocol)
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0)
	pw.String(status)
	return writeStatusFrame(conn, payload.Bytes())
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func statusPing(gateAddr, host string, protocol int) (string, error) {
	conn, err := net.DialTimeout("tcp", gateAddr, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("dial Gate Lite: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return "", fmt.Errorf("set client deadline: %w", err)
	}

	_, portText, err := net.SplitHostPort(gateAddr)
	if err != nil {
		return "", fmt.Errorf("split Gate Lite address: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", fmt.Errorf("parse Gate Lite port: %w", err)
	}

	var handshake bytes.Buffer
	pw := util.PanicWriter(&handshake)
	pw.VarInt(0)
	pw.VarInt(protocol)
	pw.String(host)
	if err := util.WriteUint16(&handshake, uint16(port)); err != nil {
		return "", fmt.Errorf("encode handshake port: %w", err)
	}
	pw.VarInt(1)
	if err := writeStatusFrame(conn, handshake.Bytes()); err != nil {
		return "", fmt.Errorf("write handshake: %w", err)
	}
	if err := writeStatusFrame(conn, []byte{0}); err != nil {
		return "", fmt.Errorf("write status request: %w", err)
	}

	id, response, err := readStatusFrame(conn)
	if err != nil {
		return "", fmt.Errorf("read status response: %w", err)
	}
	if id != 0 {
		return "", fmt.Errorf("status response packet id=%d, want 0", id)
	}
	status, err := util.ReadString(bytes.NewReader(response))
	if err != nil {
		return "", fmt.Errorf("decode status response: %w", err)
	}
	var decoded struct {
		Description struct {
			Text string `json:"text"`
		} `json:"description"`
	}
	if err := json.Unmarshal([]byte(status), &decoded); err != nil {
		return "", fmt.Errorf("decode status JSON: %w", err)
	}
	return decoded.Description.Text, nil
}

func writeStatusFrame(w io.Writer, payload []byte) error {
	var frame bytes.Buffer
	requireNoError := util.WriteVarInt(&frame, len(payload))
	if requireNoError != nil {
		return requireNoError
	}
	_, _ = frame.Write(payload)
	_, err := w.Write(frame.Bytes())
	return err
}

func readStatusFrame(r io.Reader) (int, []byte, error) {
	length, err := util.ReadVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if length <= 0 || length > 1<<20 {
		return 0, nil, fmt.Errorf("invalid frame length %d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	reader := bytes.NewReader(payload)
	id, err := util.ReadVarInt(reader)
	if err != nil {
		return 0, nil, err
	}
	data := make([]byte, reader.Len())
	_, _ = io.ReadFull(reader, data)
	return id, data, nil
}
