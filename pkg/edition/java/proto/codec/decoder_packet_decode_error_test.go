package codec

import (
	"bytes"
	"io"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
)

// A frame carrying a packet the registry knows but that cannot be decoded must
// keep two properties:
//
//   - it stays silent by default, because an untrusted peer can send it too
//     (SilentError is what the readers rely on), and
//   - the packet identity stays structured, so a caller that knows its peer is a
//     backend server can name the packet instead of parsing the message.
//
// The underlying payload error must stay reachable as well: callers classify on
// io.EOF/io.ErrUnexpectedEOF.
func TestKnownPacketDecodeFailureKeepsPacketIdentityAndStaysSilent(t *testing.T) {
	var payload bytes.Buffer
	require.NoError(t, util.WriteVarInt(&payload, 0x00)) // StatusResponse
	require.NoError(t, util.WriteVarInt(&payload, 512))  // declared string length...
	// ...with no bytes behind it, so the string read fails.

	var frame bytes.Buffer
	require.NoError(t, util.WriteVarInt(&frame, payload.Len()))
	frame.Write(payload.Bytes())

	dec := NewDecoder(bytes.NewReader(frame.Bytes()), proto.ClientBound, logr.Discard())
	dec.SetState(state.Status)
	dec.SetProtocol(version.Minecraft_1_20.Protocol)

	_, err := dec.Decode()
	require.Error(t, err)

	var decodeErr *PacketDecodeError
	require.ErrorAs(t, err, &decodeErr, "the failing packet must stay structured, not only formatted into the message")
	require.IsType(t, &packet.StatusResponse{}, decodeErr.Packet)
	require.Equal(t, proto.PacketID(0x00), decodeErr.PacketID)
	require.Equal(t, version.Minecraft_1_20.Protocol, decodeErr.Protocol)
	require.Equal(t, proto.ClientBound, decodeErr.Direction)
	require.Equal(t, 3, decodeErr.Read, "the decoder reports how much payload it consumed (packet id + string length)")
	require.Equal(t, 0, decodeErr.Unread)

	var silentErr *errs.SilentError
	require.ErrorAs(t, err, &silentErr, "an untrusted peer could send this too: it must stay silent by default")

	require.ErrorIs(t, err, io.EOF)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)

	require.Contains(t, err.Error(), "error decoding packet")
	require.Contains(t, err.Error(), "*packet.StatusResponse")
	require.Contains(t, err.Error(), "protocol: 763")
	require.Contains(t, err.Error(), "direction: ClientBound")
}
