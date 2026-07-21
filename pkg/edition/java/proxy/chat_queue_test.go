package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
)

func TestChatStateUpdateFromMessageDivergesAfterGateHoldsAcknowledgements(t *testing.T) {
	directState := &ChatState{}
	proxiedState := &ChatState{}

	const (
		heldAcknowledgements = 3
		clientOffset         = 7
	)
	direct := directState.UpdateFromMessage(nil, &chat.LastSeenMessages{Offset: clientOffset})
	proxiedState.AccumulateAckCount(heldAcknowledgements)
	proxied := proxiedState.UpdateFromMessage(nil, &chat.LastSeenMessages{Offset: clientOffset})

	require.NotNil(t, direct)
	require.NotNil(t, proxied)
	require.Equal(t, clientOffset, direct.Offset)
	require.Equal(t, clientOffset+heldAcknowledgements, proxied.Offset)
}
