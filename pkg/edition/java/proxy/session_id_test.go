package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/util/uuid"
)

func TestProxySessionIDSharedUntilProxyEmpties(t *testing.T) {
	p := &Proxy{
		playerIDs: map[uuid.UUID]*connectedPlayer{},
	}

	first := p.sessionID()
	require.Equal(t, first, p.sessionID())

	playerID := uuid.New()
	p.playerIDs[playerID] = nil
	p.resetSessionIDIfEmpty()
	require.Equal(t, first, p.sessionID())

	delete(p.playerIDs, playerID)
	p.resetSessionIDIfEmpty()
	require.NotEqual(t, first, p.sessionID())
}
