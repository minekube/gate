package queue

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// dummyPlayPacket is a packet that is NOT registered in CONFIG state,
// so it will be queued by PlayPacketQueue.
type dummyPlayPacket struct{}

func (d *dummyPlayPacket) Encode(*proto.PacketContext, io.Writer) error { return nil }
func (d *dummyPlayPacket) Decode(*proto.PacketContext, io.Reader) error { return nil }

func TestPlayPacketQueue_QueueAndRelease(t *testing.T) {
	q := NewPlayPacketQueue(version.Minecraft_1_21_11.Protocol, proto.ClientBound)

	queued, err := q.Queue(&dummyPlayPacket{})
	require.NoError(t, err)
	require.True(t, queued)

	var released int
	err = q.ReleaseQueue(
		func(proto.Packet) error { released++; return nil },
		func() error { return nil },
	)
	require.NoError(t, err)
	require.Equal(t, 1, released)
}

func TestPlayPacketQueue_MaxLimit(t *testing.T) {
	q := NewPlayPacketQueue(version.Minecraft_1_21_11.Protocol, proto.ClientBound)

	// Fill up to the limit
	for i := 0; i < maxQueueLen; i++ {
		queued, err := q.Queue(&dummyPlayPacket{})
		require.NoError(t, err)
		require.True(t, queued, "packet %d should be queued", i)
	}

	// Next one should fail
	_, err := q.Queue(&dummyPlayPacket{})
	require.ErrorIs(t, err, ErrQueueFull)
}

func TestPlayPacketQueue_NilSafe(t *testing.T) {
	var q *PlayPacketQueue
	queued, err := q.Queue(&dummyPlayPacket{})
	require.NoError(t, err)
	require.False(t, queued)

	err = q.ReleaseQueue(nil, nil)
	require.NoError(t, err)
}
