package gate

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/configs"
	"go.minekube.com/gate/pkg/gate/config"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/runtime/process"
	connectcfg "go.minekube.com/gate/pkg/util/connectutil/config"
)

// These tests cover the lifecycle of the Connect runtime Gate starts
// (pkg/gate/connect.go): a Connect client that is still running after Gate
// reported "stopped" keeps reconnecting and (re)writes its token file, which is
// what made the startup smoke test flake on loaded CI runners with
// "TempDir RemoveAll cleanup: directory not empty" — the write raced the
// removal of the test's temp dir because nothing joined the goroutine.
//
// The runtime is replaced by one whose shutdown the test holds, so the
// interleaving is forced instead of raced: the runtime is provably still
// running when the test asserts.

// connectJoinProbeWindow is how long a test waits to observe that Gate has NOT
// stopped while its Connect runtime is held. It is orders of magnitude above
// the sub-millisecond shutdown of Gate's other components, and on the correct
// implementation it only ever elapses.
const connectJoinProbeWindow = time.Second

// stubConnectRuntime replaces the Connect runtime factory for one test.
func stubConnectRuntime(
	t *testing.T,
	factory func(connectcfg.Config, connectcfg.Instance) (process.Runnable, error),
) {
	t.Helper()
	original := newConnectRuntime
	newConnectRuntime = factory
	t.Cleanup(func() { newConnectRuntime = original })
}

// gateRun is a Gate started by a test, with a start result the test can observe
// so it can assert whether Gate has already reported its shutdown.
type gateRun struct {
	*Gate
	Cancel  context.CancelFunc
	started chan error
}

// startGate boots Gate with the given config and event manager, waits until the
// proxy listens and stops the Gate on test cleanup.
func startGate(t *testing.T, cfg *config.Config, events event.Manager) *gateRun {
	t.Helper()

	g, err := New(Options{Config: cfg, EventMgr: events})
	require.NoError(t, err, "Gate must wire up all components of a valid config")

	ctx, cancel := context.WithCancel(context.Background())
	run := &gateRun{Gate: g, Cancel: cancel, started: make(chan error, 1)}
	go func() { run.started <- g.Start(ctx) }()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-run.started:
			require.NoError(t, err, "Gate must shut down without error")
		case <-time.After(shutdownTimeout):
			t.Error("Gate did not shut down")
		}
	})

	requireDialable(t, cfg.Config.Bind)
	return run
}

// stopResult reports Gate's Start error if Gate stopped within timeout.
func (r *gateRun) stopResult(timeout time.Duration) (error, bool) {
	select {
	case err := <-r.started:
		r.started <- err // the cleanup waits on this channel too
		return err, true
	case <-time.After(timeout):
		return nil, false
	}
}

// stepLog records the ordered lifecycle steps a test-driven runtime reports, so
// a test can assert ordering instead of sleeping.
type stepLog struct {
	mu    sync.Mutex
	steps []string
}

func (l *stepLog) add(step string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.steps = append(l.steps, step)
}

func (l *stepLog) list() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.steps...)
}

// waitFor waits until step was recorded and returns what was recorded so far.
func (l *stepLog) waitFor(t *testing.T, step string, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		steps := l.list()
		for _, s := range steps {
			if s == step {
				return steps
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("step %q was not recorded within %s (recorded: %v)", step, timeout, steps)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestConnectRuntimeIsJoinedOnShutdown proves Gate's shutdown waits (bounded)
// for the Connect runtime it started: a stopped Gate must not leave a connector
// running that can still dial the watch service or rewrite its token file.
func TestConnectRuntimeIsJoinedOnShutdown(t *testing.T) {
	var (
		started  = make(chan struct{})
		stopping = make(chan struct{})
		release  = make(chan struct{})
	)
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()

	stubConnectRuntime(t, func(connectcfg.Config, connectcfg.Instance) (process.Runnable, error) {
		return process.RunnableFunc(func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(stopping)
			<-release // held by the test: the runtime cannot stop on its own
			return nil
		}), nil
	})

	cfg := loadTestConfig(t, configs.MinimalConfigBytes)
	cfg.Config.Bind = reserveAddr(t)
	cfg.Connect.Enabled = true
	cfg.Connect.Name = "gate-connect-join-test"

	run := startGate(t, cfg, event.New())
	select {
	case <-started:
	case <-time.After(startupTimeout):
		t.Fatal("Gate did not start its Connect runtime")
	}

	run.Cancel()
	select {
	case <-stopping:
	case <-time.After(startupTimeout):
		t.Fatal("Gate never asked its Connect runtime to stop")
	}

	// The runtime is provably still running (the test holds it), so Gate must
	// not have reported a stopped instance yet.
	if err, stopped := run.stopResult(connectJoinProbeWindow); stopped {
		t.Fatalf("Gate stopped while its Connect runtime was still running: %v", err)
	}

	releaseOnce()
	err, stopped := run.stopResult(shutdownTimeout)
	require.True(t, stopped, "Gate must stop once its Connect runtime returned")
	require.NoError(t, err, "Gate must shut down without error")
}

// TestConnectReloadStopsPreviousRuntimeFirst proves replacing the Connect config
// replaces the connector instead of overlapping it: the superseded runtime must
// have stopped before its replacement is built, otherwise two connectors watch
// the same endpoint (and provision the same token file) at once.
func TestConnectReloadStopsPreviousRuntimeFirst(t *testing.T) {
	var runtimes atomic.Int32
	var (
		steps       stepLog
		created     = make(chan struct{}, 4)
		release     = make(chan struct{})
		releaseOnce = sync.OnceFunc(func() { close(release) })
	)
	defer releaseOnce()

	stubConnectRuntime(t, func(connectcfg.Config, connectcfg.Instance) (process.Runnable, error) {
		n := int(runtimes.Add(1))
		steps.add(fmt.Sprintf("create %d", n))
		if n == 2 {
			select {
			case created <- struct{}{}:
			default:
			}
		}
		return process.RunnableFunc(func(ctx context.Context) error {
			steps.add(fmt.Sprintf("start %d", n))
			<-ctx.Done()
			steps.add(fmt.Sprintf("stopping %d", n))
			<-release // held by the test until it has checked the ordering
			steps.add(fmt.Sprintf("stopped %d", n))
			return nil
		}), nil
	})

	cfg := loadTestConfig(t, configs.MinimalConfigBytes)
	cfg.Config.Bind = reserveAddr(t)
	cfg.Connect.Enabled = true
	cfg.Connect.Name = "gate-connect-reload-a"

	events := event.New()
	startGate(t, cfg, events)
	steps.waitFor(t, "start 1", startupTimeout)

	// A changed Connect config reloads the connector.
	updated := *cfg
	updated.Connect.Name = "gate-connect-reload-b"
	reloadDone := make(chan struct{})
	go func() {
		defer close(reloadDone)
		reload.FireConfigUpdate(events, &updated, cfg)
	}()

	// The old runtime is asked to stop and is held there by the test.
	steps.waitFor(t, "stopping 1", startupTimeout)

	// While the old runtime is still running, the reload must not have built
	// its replacement.
	select {
	case <-created:
		t.Fatalf("the Connect runtime was replaced while the previous one was still running (steps: %v)", steps.list())
	case <-time.After(connectJoinProbeWindow):
	}

	releaseOnce()
	steps.waitFor(t, "stopped 1", shutdownTimeout)
	steps.waitFor(t, "start 2", shutdownTimeout)
	select {
	case <-reloadDone:
	case <-time.After(shutdownTimeout):
		t.Fatal("the Connect reload did not complete after the previous runtime stopped")
	}

	require.Equal(t,
		[]string{"create 1", "start 1", "stopping 1", "stopped 1", "create 2", "start 2"},
		steps.list(),
		"the previous Connect runtime must stop before its replacement starts")
}
