package gate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/util/configutil"
)

func TestGateConfigSnapshotVersionAndConditionalApply(t *testing.T) {
	initial := liveReloadConfig()
	g, err := New(Options{Config: initial})
	require.NoError(t, err)

	snapshot, version, err := g.ConfigSnapshot()
	require.NoError(t, err)
	require.NotEmpty(t, version)
	require.Equal(t, initial.Config.Bind, snapshot.Config.Bind)
	require.Equal(t, initial.Config.Lite.Routes, snapshot.Config.Lite.Routes)

	_, sameVersion, err := g.ConfigSnapshot()
	require.NoError(t, err)
	require.Equal(t, version, sameVersion)

	candidate := *snapshot
	candidate.Config.Lite.Routes = append(candidate.Config.Lite.Routes[:0:0], snapshot.Config.Lite.Routes...)
	candidate.Config.Lite.Routes[0].CachePingTTL = configutil.Duration(time.Minute)

	stale := g.ApplyLiveConfigIfVersion(&candidate, "stale")
	require.Equal(t, "precondition_failed", stale.Code)
	require.False(t, stale.Applied)

	unchanged, currentVersion, err := g.ConfigSnapshot()
	require.NoError(t, err)
	require.Equal(t, version, currentVersion)
	require.Equal(t, initial.Config.Lite.Routes, unchanged.Config.Lite.Routes)

	applied := g.ApplyLiveConfigIfVersion(&candidate, version)
	require.True(t, applied.Applied)
	require.Equal(t, "applied", applied.Code)
	require.NotEmpty(t, applied.Version)
	require.NotEqual(t, version, applied.Version)
}
