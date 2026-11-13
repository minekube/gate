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

	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// truncatedStringFrame returns a wire frame for a packet kind that IS registered
// for the decoder's state/protocol, whose payload announces a string longer than
// the frame carries. The decoder therefore knows the packet type before it fails
// to decode it, which is what makes the failure nameable at all.
func truncatedStringFrame(t *testing.T, packetID, declaredStringLen int) []byte {
	t.Helper()

	var payload bytes.Buffer
	require.NoError(t, util.WriteVarInt(&payload, packetID))
	require.NoError(t, util.WriteVarInt(&payload, declaredStringLen))
	// Deliberately no string bytes: the declared length is what fails the decode.

	var frame bytes.Buffer
	require.NoError(t, util.WriteVarInt(&frame, payload.Len()))
	frame.Write(payload.Bytes())
	return frame.Bytes()
}

// readTruncatedKnownPacket feeds the reader one known-but-undecodable packet and
// returns everything logged at the verbosity an operator runs with by default
// (V(1) debug lines are dropped) together with the closing error.
func readTruncatedKnownPacket(
	t *testing.T,
	direction proto.Direction,
	registry *state.Registry,
	protocol proto.Protocol,
	packetID int,
) ([]string, error) {
	t.Helper()

	frame := truncatedStringFrame(t, packetID, 512)

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
		_, _ = remote.Write(frame)
	}()

	r := NewReader(local, direction, time.Second, log)
	r.SetState(registry)
	r.SetProtocol(protocol)
	_, err := r.ReadPacket()
	require.Error(t, err, "a known packet whose payload cannot be decoded must fail the read and close the connection")

	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), lines...), err
}

// A backend server (the reader of a ClientBound stream) sending a packet that
// Gate recognises but cannot decode is operator-actionable in exactly the same
// way an oversized frame is: Gate just closes the socket, so the backend's own
// log stays empty, the player only sees "Internal server connection error", and
// on a Connect tunnel there is no other place the reason could be recorded. Both
// ends being Minekube-run makes the close unattributable *by construction* unless
// this line names the packet that ended the session.
//
// The client side of the same failure stays silent by policy
// (TestReaderKeepsClientPacketDecodeFailureSilent), because an untrusted peer can
// open a connection and would turn an INFO line into a log flood primitive.
func TestReaderLogsBackendPacketDecodeFailure(t *testing.T) {
	lines, err := readTruncatedKnownPacket(
		t, proto.ClientBound, state.Status, version.Minecraft_1_20.Protocol, 0x00) // StatusResponse

	require.Contains(t, err.Error(), "error decoding packet", "the frame must exercise the known-packet decode path")

	require.NotEmpty(t, lines, "a backend packet that cannot be decoded must be visible at default verbosity")
	logged := strings.Join(lines, "\n")
	require.Contains(t, logged, "could not be decoded")
	require.Contains(t, logged, "StatusResponse", "the packet type that ended the session must be named")
	require.Contains(t, logged, "ClientBound", "the direction must be named")
	require.Contains(t, logged, "763", "the protocol that ended the session must be named")
	require.Contains(t, logged, "peer", "the backend peer must be named")
}

// The same failure from a client is untrusted traffic: anyone can open a
// connection, so it stays a quiet close at default verbosity.
func TestReaderKeepsClientPacketDecodeFailureSilent(t *testing.T) {
	lines, err := readTruncatedKnownPacket(
		t, proto.ServerBound, state.Login, version.Minecraft_1_20.Protocol, 0x00) // ServerLogin

	require.Contains(t, err.Error(), "error decoding packet", "the frame must exercise the known-packet decode path")

	require.Empty(t, lines, "a client packet that cannot be decoded must not be logged at default verbosity")
}
