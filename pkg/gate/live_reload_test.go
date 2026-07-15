package gate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/gate/config"
	"go.minekube.com/gate/pkg/util/configutil"
)

func TestGateApplyLiveConfigPublishesOnlyValidatedLiteRouteSnapshot(t *testing.T) {
	initial := liveReloadConfig()
	g, err := New(Options{Config: initial})
	require.NoError(t, err)

	candidate := *initial
	candidate.Config.Lite.Routes = append([]liteconfig.Route(nil), initial.Config.Lite.Routes...)
	candidate.Config.Lite.Routes[0].CachePingTTL = configutil.Duration(time.Minute)

	result := g.ApplyLiveConfig(&candidate)
	require.True(t, result.Applied)
	require.True(t, result.CacheInvalidated)
	require.Equal(t, configutil.Duration(time.Minute), g.Java().Config().Lite.Routes[0].CachePingTTL)
}

func TestGateApplyLiveConfigRejectsInvalidCandidateAndRetainsLastKnownGood(t *testing.T) {
	initial := liveReloadConfig()
	g, err := New(Options{Config: initial})
	require.NoError(t, err)

	candidate := *initial
	candidate.Config.Lite.Routes = []liteconfig.Route{{Host: []string{"play.example.test"}}}

	result := g.ApplyLiveConfig(&candidate)
	require.False(t, result.Applied)
	require.Equal(t, "invalid", result.Code)
	require.Equal(t, initial.Config.Lite.Routes, g.Java().Config().Lite.Routes)
}

func TestGateApplyLiveConfigRejectsUnsupportedChangesWithoutDisconnectingOrPublishing(t *testing.T) {
	initial := liveReloadConfig()
	g, err := New(Options{Config: initial})
	require.NoError(t, err)

	candidate := *initial
	candidate.Config.Bind = "127.0.0.1:25566"

	result := g.ApplyLiveConfig(&candidate)
	require.False(t, result.Applied)
	require.Equal(t, "unsupported", result.Code)
	require.Equal(t, initial.Config.Bind, g.Java().Config().Bind)
}

func TestGateApplyLiveConfigTreatsUnchangedCandidateAsNoOp(t *testing.T) {
	initial := liveReloadConfig()
	g, err := New(Options{Config: initial})
	require.NoError(t, err)

	result := g.ApplyLiveConfig(initial)
	require.True(t, result.Unchanged)
	require.False(t, result.Applied)
}

func TestGateApplyLiveConfigRejectsSemanticFailureAndConcurrentReadersNeverSeePartialConfig(t *testing.T) {
	initial := liveReloadConfig()
	g, err := New(Options{Config: initial})
	require.NoError(t, err)

	invalid := *initial
	invalid.Config.Lite.Routes = append([]liteconfig.Route(nil), initial.Config.Lite.Routes...)
	invalid.Config.Lite.Routes[0].Strategy = "not-a-strategy"
	require.Equal(t, "invalid", g.ApplyLiveConfig(&invalid).Code)

	var readers sync.WaitGroup
	for range 32 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range 200 {
				cfg := g.Java().Config()
				require.NotEmpty(t, cfg.Lite.Routes)
				require.NotEmpty(t, cfg.Lite.Routes[0].Backend)
			}
		}()
	}
	for i := range 200 {
		candidate := *initial
		candidate.Config.Lite.Routes = append([]liteconfig.Route(nil), initial.Config.Lite.Routes...)
		candidate.Config.Lite.Routes[0].CachePingTTL = configutil.Duration(time.Duration(i+1) * time.Second)
		require.True(t, g.ApplyLiveConfig(&candidate).Applied)
	}
	readers.Wait()
}

func TestValidateConfigFileSyntaxRejectsPartialAndUnknownCandidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("config:\n  lite: ["), 0o600))
	require.Error(t, validateConfigFileSyntax(path))

	require.NoError(t, os.WriteFile(path, []byte("config:\n  unknownOption: true\n"), 0o600))
	require.Error(t, validateConfigFileSyntax(path))
}

func liveReloadConfig() *config.Config {
	c := config.DefaultConfig
	c.Config.Bind = "127.0.0.1:25565"
	c.Config.Lite.Enabled = true
	c.Config.Lite.Routes = []liteconfig.Route{{
		Host:         []string{"play.example.test"},
		Backend:      []string{"backend.example.test:25565"},
		CachePingTTL: configutil.Duration(30 * time.Second),
	}}
	return &c
}
