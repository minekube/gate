package config

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/runtime/process"
)

// sleepRecorder replaces the package sleep/jitter hooks with a recorder that
// returns immediately and records the requested delays, so tests can assert
// backoff behavior without actually waiting. The originals are restored on
// test cleanup.
type sleepRecorder struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (r *sleepRecorder) install(t *testing.T) {
	t.Helper()
	origSleep, origJitter := sleep, jitter
	sleep = func(_ context.Context, d time.Duration) {
		r.mu.Lock()
		r.delays = append(r.delays, d)
		r.mu.Unlock()
	}
	jitter = func(d time.Duration) time.Duration { return d } // deterministic
	t.Cleanup(func() { sleep, jitter = origSleep, origJitter })
}

func (r *sleepRecorder) snapshot() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.delays...)
}

// TestRetryingRunnableBacksOffOnAuthRejected proves the watch retry loop
// backs off exponentially on 401 (5s → 10s → ...) instead of the historical
// fixed 5s interval, then switches to a cold recovery probe without making a
// token rotation or released endpoint name require a Gate restart.
func TestRetryingRunnableBacksOffOnAuthRejected(t *testing.T) {
	rec := &sleepRecorder{}
	rec.install(t)

	var attempts int
	r := process.RunnableFunc(func(context.Context) error {
		attempts++
		if attempts == 5 {
			return nil
		}
		return &authRejectedError{endpoint: "displaced-1", err: errors.New("401 unauthorized")}
	})

	err := retryingRunnable(r).Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, 5, attempts)
	require.Equal(t, []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Minute, 15 * time.Minute}, rec.snapshot(),
		"must back off exponentially before entering the cold recovery cadence")
}

// TestRetryingRunnableDoesNotHammerPermanentlyRejected proves a
// repeatedly rejected (endpoint, token) is not hammered, but remains able to
// recover automatically when the endpoint or token becomes valid.
func TestRetryingRunnableDoesNotHammerPermanentlyRejected(t *testing.T) {
	rec := &sleepRecorder{}
	rec.install(t)

	var attempts int
	r := process.RunnableFunc(func(context.Context) error {
		attempts++
		if attempts == 20 {
			return nil
		}
		return &authRejectedError{endpoint: "org-owned", err: errors.New("401 unauthorized")}
	})

	err := retryingRunnable(r).Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, 20, attempts)
	delays := rec.snapshot()
	require.Equal(t, []time.Duration{5 * time.Second, 10 * time.Second}, delays[:2])
	for _, delay := range delays[2:] {
		require.Equal(t, authRejectedRetryAfter, delay)
	}
}

// TestRetryingRunnableExponentialBackoffOnTransientError proves non-401
// failures keep retrying with exponential backoff (and eventually succeed),
// so transient errors are not treated as terminal.
func TestRetryingRunnableExponentialBackoffOnTransientError(t *testing.T) {
	rec := &sleepRecorder{}
	rec.install(t)

	var attempts int
	r := process.RunnableFunc(func(context.Context) error {
		attempts++
		if attempts < 4 {
			return errors.New("transient network error")
		}
		return nil
	})

	require.NoError(t, retryingRunnable(r).Start(context.Background()))
	require.Equal(t, 4, attempts)
	require.Equal(t, []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second}, rec.snapshot())
}

// TestBackoffDelayCappedAtMax proves the exponential backoff doubles and is
// capped at retryAfterMax.
func TestBackoffDelayCappedAtMax(t *testing.T) {
	require.Equal(t, 5*time.Second, backoffDelay(1))
	require.Equal(t, 10*time.Second, backoffDelay(2))
	require.Equal(t, 20*time.Second, backoffDelay(3))
	require.Equal(t, retryAfterMax, backoffDelay(100))
	require.Equal(t, retryAfterMax, backoffDelay(1000))
}

// TestJitterBounds proves jitter stays within ±20% of the base delay.
func TestJitterBounds(t *testing.T) {
	for _, base := range []time.Duration{5 * time.Second, 30 * time.Second, retryAfterMax} {
		for i := 0; i < 100; i++ {
			d := jitter(base)
			require.GreaterOrEqual(t, d, base*4/5, "jitter must not undercut base by more than 20%%")
			require.LessOrEqual(t, d, base*6/5, "jitter must not exceed base by more than 20%%")
		}
	}
}

// TestRetryingRunnableResetsBackoffAfterHealthySession proves a session that
// stayed up longer than healthySessionDuration resets the backoff, so a
// disconnect after long uptime (e.g. server restart) reconnects fast instead
// of inheriting a grown delay.
func TestRetryingRunnableResetsBackoffAfterHealthySession(t *testing.T) {
	rec := &sleepRecorder{}
	rec.install(t)
	origHealthy := healthySessionDuration
	healthySessionDuration = 10 * time.Millisecond
	t.Cleanup(func() { healthySessionDuration = origHealthy })

	var attempts int
	r := process.RunnableFunc(func(ctx context.Context) error {
		attempts++
		// First session: stay up longer than healthySessionDuration, then
		// fail. Later sessions: fail immediately.
		if attempts == 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
			return errors.New("server restarted after long uptime")
		}
		return errors.New("transient")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- retryingRunnable(r).Start(ctx) }()

	// After the healthy session, the first retry must be the base delay
	// again (not a grown one).
	require.Eventually(t, func() bool { return len(rec.snapshot()) >= 2 }, time.Second, 10*time.Millisecond)
	delays := rec.snapshot()
	require.Equal(t, 5*time.Second, delays[0], "first retry after healthy session must restart at base backoff")
	cancel()
	<-done
}

// TestWatchClientColdProbesOnPersistent401 is the end-to-end regression: the
// real watch client enters the cold cadence after repeated HTTP 401s and stops
// promptly when its lifecycle context is canceled.
func TestWatchClientColdProbesOnPersistent401(t *testing.T) {
	rec := &sleepRecorder{}
	rec.install(t)

	var (
		mu       sync.Mutex
		attempts int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := Config{
		WatchServiceAddr: "ws://" + strings.TrimPrefix(srv.URL, "http://"),
		Name:             "perma-401-e2e",
		TokenFilePath:    filepath.Join(t.TempDir(), "token.json"),
	}
	runnable, err := connectClient(c, connHandlerFunc(func(net.Conn) {}))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	originalSleep := sleep
	sleep = func(ctx context.Context, d time.Duration) {
		originalSleep(ctx, d)
		if len(rec.snapshot()) == 4 {
			cancel()
		}
	}
	err = runnable.Start(ctx)
	require.NoError(t, err)

	mu.Lock()
	got := attempts
	mu.Unlock()
	require.Equal(t, 4, got)
	require.Equal(t, []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Minute, 15 * time.Minute}, rec.snapshot())
}
