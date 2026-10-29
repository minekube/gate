//go:build !musl

package geyser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
	geyserlite "go.minekube.com/geyserlite"
)

// GeyserLite's authenticated native framing publishes an open notification for
// each transport, even when this Floodgate integration makes no assignments.
// A generation must survive more than the upstream queue's 256 lifetime opens.
func TestLiteManagedRunnerDrainsFloodgateConnectionNotifications(t *testing.T) {
	opened := make(chan geyserlite.ConnectionOpen, 256)
	server := &notificationChurnServer{
		fakeGeyserliteServer: fakeGeyserliteServer{started: make(chan struct{}), opened: opened},
		out:                  opened,
	}
	runner := notificationRunner(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("managed Floodgate runtime stopped after %d opens: %v", server.emitted.Load(), err)
	}
	if got := server.emitted.Load(); got != 1024 {
		t.Fatalf("processed %d opens, want 1024", got)
	}
	done := runner.Done()
	runner.Stop()
	select {
	case <-done:
	default:
		t.Fatal("Stop returned before runtime and notification drain finished")
	}
}

func TestLiteManagedRunnerJoinsNotificationDrainOnRuntimeExit(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(map[bool]string{false: "open channel", true: "closed channel"}[closed], func(t *testing.T) {
			opened := make(chan geyserlite.ConnectionOpen)
			if closed {
				close(opened)
			}
			server := &fakeGeyserliteServer{started: make(chan struct{}), crash: make(chan error, 1), opened: opened}
			server.healthy.Store(true)
			runner := notificationRunner(t, server)
			if err := runner.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			done := runner.Done()
			want := errors.New("runtime exit fixture")
			server.crash <- want
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("runtime exit did not join the notification drain")
			}
			if !errors.Is(runner.Err(), want) {
				t.Fatalf("runtime error = %v, want %v", runner.Err(), want)
			}
			if !closed {
				select {
				case opened <- geyserlite.ConnectionOpen{}:
					t.Fatal("notification reader survived runtime exit")
				default:
				}
			}
		})
	}
}

func notificationRunner(t *testing.T, server geyserliteServer) *liteManagedRunner {
	t.Helper()
	key := filepath.Join(t.TempDir(), "floodgate.key")
	if err := os.WriteFile(key, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := newLiteManagedRunner(&config.Config{
		FloodgateKeyPath: key,
		GeyserListenAddr: "localhost:25567",
		Managed: &config.ManagedGeyser{ConfigOverrides: map[string]any{
			"bedrock": map[string]any{"port": freeUDPPort(t)},
		}},
	})
	runner.newServer = func(options geyserlite.Options) (geyserliteServer, error) {
		if options.AuthType != geyserlite.Floodgate {
			t.Fatalf("notification drain must not change Floodgate authentication: %v", options.AuthType)
		}
		return server, nil
	}
	t.Cleanup(runner.Stop)
	return runner
}

type notificationChurnServer struct {
	fakeGeyserliteServer
	out     chan<- geyserlite.ConnectionOpen
	emitted atomic.Int32
}

func (s *notificationChurnServer) Start(ctx context.Context) error {
	close(s.started)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for handle := uint64(1); handle <= 1024; handle++ {
		select {
		case s.out <- geyserlite.ConnectionOpen{Generation: 1, ConnectionHandle: handle}:
			s.emitted.Add(1)
		case <-deadline.C:
			return geyserlite.ErrIngressOverflow
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.healthy.Store(true)
	<-ctx.Done()
	return ctx.Err()
}
