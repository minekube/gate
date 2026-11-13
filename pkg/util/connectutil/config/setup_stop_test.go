package config

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWatchClientDoesNotProvisionTokenAfterStopRequest proves a watch client
// that is asked to stop while backing off does not start another attempt: an
// attempt provisions the token file (loadToken creates and rewrites it), so a
// stop request could otherwise still write into Gate's data dir. That write is
// what made pkg/gate's startup smoke test flake on loaded CI runners with
// "TempDir RemoveAll cleanup: directory not empty".
//
// The backoff is held by the test, so the client is provably still backing off
// when the stop is requested and the exchange is ordered instead of raced.
func TestWatchClientDoesNotProvisionTokenAfterStopRequest(t *testing.T) {
	// Keep the token handling hermetic against a developer's environment.
	t.Setenv("CONNECT_TOKEN", "")

	tokenFile := filepath.Join(t.TempDir(), "connect.json")

	originalSleep := sleep
	inBackoff := make(chan struct{}, 1)
	sleep = func(ctx context.Context, _ time.Duration) {
		select {
		case inBackoff <- struct{}{}:
		default:
		}
		<-ctx.Done() // hold the backoff until the stop request arrives
	}
	t.Cleanup(func() { sleep = originalSleep })

	c := Config{
		// Nothing listens there, so the first attempt fails fast and the
		// client enters its backoff.
		WatchServiceAddr: "ws://" + unusedTCPAddr(t) + "/watch",
		Name:             "stop-request-token-test",
		TokenFilePath:    tokenFile,
	}
	runnable, err := connectClient(c, connHandlerFunc(func(net.Conn) {}))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runnable.Start(ctx) }()

	select {
	case <-inBackoff:
	case <-time.After(10 * time.Second):
		t.Fatal("the watch client did not start backing off")
	}
	// The first attempt provisioned the token file, and the client is now
	// provably held in its backoff.
	require.FileExists(t, tokenFile)
	require.NoError(t, os.Remove(tokenFile))

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err, "a watch client that was asked to stop must return without error")
	case <-time.After(10 * time.Second):
		t.Fatal("the watch client did not stop")
	}

	require.NoFileExists(t, tokenFile,
		"the watch client provisioned its token file after it was asked to stop")
}

// unusedTCPAddr returns a localhost address nothing listens on.
func unusedTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}
