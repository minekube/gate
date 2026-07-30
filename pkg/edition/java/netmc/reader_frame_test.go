package netmc

import (
	"bytes"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

// readOversizedFrame feeds the reader a frame header announcing a length above
// codec.MaximumFrameLength and returns everything logged at the verbosity an
// operator runs with by default (V(1) debug lines are dropped).
func readOversizedFrame(t *testing.T, direction proto.Direction) []string {
	t.Helper()

	local, remote := net.Pipe()
	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
	})

	var mu sync.Mutex
	var lines []string
	log := funcr.New(func(prefix, args string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, prefix+" "+args)
	}, funcr.Options{Verbosity: 0})

	go func() {
		// Only the length prefix is needed: the frame is rejected on its
		// announced length, before any payload is read.
		var frame bytes.Buffer
		_ = util.WriteVarInt(&frame, codec.MaximumFrameLength+1)
		_, _ = remote.Write(frame.Bytes())
	}()

	_, err := NewReader(local, direction, time.Second, log).ReadPacket()
	require.Error(t, err, "an oversized frame must fail the read and close the connection")

	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), lines...)
}

// A backend server sending an oversized frame is operator-actionable: Gate just
// closes the socket, so the backend's own log stays empty and the player only
// sees "unable to connect". That diagnosis has to be visible at default
// verbosity, with the peer and the observed length.
//
// See https://github.com/minekube/gate/issues/930.
func TestReaderLogsOversizedBackendFrame(t *testing.T) {
	lines := readOversizedFrame(t, proto.ClientBound)
	require.NotEmpty(t, lines, "an oversized backend frame must be visible at default verbosity")

	logged := strings.Join(lines, "\n")
	require.Contains(t, logged, "larger than the maximum allowed")
	require.Contains(t, logged, "peer")
	require.Contains(t, logged, "frameLength")
	require.Contains(t, logged, "2097152", "the observed frame length must be reported")
	require.Contains(t, logged, "2097151", "the maximum must be reported alongside it")
}

// A client is untrusted and anyone can open a connection, so the same oversized
// frame from a client stays a quiet close — otherwise it becomes a log flood
// primitive.
func TestReaderDoesNotLogOversizedClientFrame(t *testing.T) {
	lines := readOversizedFrame(t, proto.ServerBound)
	require.Empty(t, lines, "an oversized client frame must not be logged at default verbosity")
}
