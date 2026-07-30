package gate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

func TestConfigHandlerReportsWildcardLiteConnectionsOnce(t *testing.T) {
	cfg := liveReloadConfig()
	cfg.Config.Lite.Routes = []liteconfig.Route{{
		Host:    []string{"*.example.test"},
		Backend: []string{"$1.backend.example.test:25565", "$1.backend.example.test:25565"},
	}}
	g, err := New(Options{Config: cfg})
	require.NoError(t, err)
	handler := NewConfigHandler(g, "")

	sm := g.Java().Lite().StrategyManager()
	release1 := sm.TrackConnection("*.example.test", "player.backend.example.test")
	release2 := sm.TrackConnection("*.example.test", "player.backend.example.test:25565")
	defer release1()
	defer release2()

	status, err := handler.GetStatus(context.Background(), &pb.GetStatusRequest{})
	require.NoError(t, err)
	require.Equal(t, int32(2), status.GetLite().GetConnections())

	status, err = handler.GetStatus(context.Background(), &pb.GetStatusRequest{})
	require.NoError(t, err)
	require.Equal(t, int32(2), status.GetLite().GetConnections())
}
