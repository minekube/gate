package netmc

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// availableCommandsFrame returns a play-state frame carrying an AvailableCommands
// (0x10) payload whose tree names an argument type outside the vanilla 763 table,
// which is what a modded backend sends.
func availableCommandsFrame(t *testing.T) (frame, payload []byte) {
	t.Helper()

	var body bytes.Buffer
	require.NoError(t, util.WriteVarInt(&body, 0x10)) // AvailableCommands
	body.Write([]byte{
		3,          // node count
		0x00, 1, 1, // root -> 1
		0x02 | 0x04, 0, 6, 'c', 'u', 's', 't', 'o', 'm', 99, // argument node, id 99 is not vanilla
		0x01 | 0x04, 0, 5, 'o', 't', 'h', 'e', 'r', // literal, executable
		0, // root index
	})

	var framed bytes.Buffer
	require.NoError(t, util.WriteVarInt(&framed, body.Len()))
	framed.Write(body.Bytes())
	return framed.Bytes(), body.Bytes()
}

// A backend command tree the proxy cannot decode must not close the session: the
// read succeeds and the original payload is what gets forwarded to the player.
// Regression: a modded Forge 1.20.1 join through the Connect edge was kicked
// ~0.8 s after placement because this decode error closed the backend connection.
func TestReaderKeepsSessionOnUndecodableCommandTree(t *testing.T) {
	frame, payload := availableCommandsFrame(t)

	local, remote := net.Pipe()
	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
	})
	go func() {
		_, _ = remote.Write(frame)
	}()

	var mu sync.Mutex
	var lines []string
	log := funcr.New(func(prefix, args string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, prefix+" "+args)
	}, funcr.Options{Verbosity: 0})

	r := NewReader(local, proto.ClientBound, time.Second, log)
	r.SetState(state.Play)
	r.SetProtocol(version.Minecraft_1_20.Protocol)

	pc, err := r.ReadPacket()
	require.NoError(t, err, "an undecodable backend command tree must not close the session")
	require.NotNil(t, pc)

	cmd, ok := pc.Packet.(*packet.AvailableCommands)
	require.True(t, ok, "the packet must still be recognised as AvailableCommands, got %T", pc.Packet)
	require.NotEmpty(t, cmd.Raw, "the undecodable tree must be carried as raw bytes for forwarding")
	require.Error(t, cmd.DecodeErr)

	// The proxy forwards pc.Payload (the backend's bytes, packet id included).
	require.Equal(t, payload, pc.Payload, "the client must receive exactly the backend's command tree")

	mu.Lock()
	defer mu.Unlock()
	require.Empty(t, lines, "a session that survives must not log a closing error at default verbosity")
}
