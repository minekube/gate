package gate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
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

func TestGateApplyLiveConfigOwnsPublishedRouteSnapshot(t *testing.T) {
	initial := liveReloadConfig()
	g, err := New(Options{Config: initial})
	require.NoError(t, err)

	candidate := *initial
	candidate.Config.Lite.Routes = append([]liteconfig.Route(nil), initial.Config.Lite.Routes...)
	candidate.Config.Lite.Routes[0].Host = append([]string(nil), initial.Config.Lite.Routes[0].Host...)
	candidate.Config.Lite.Routes[0].Backend = append([]string(nil), initial.Config.Lite.Routes[0].Backend...)
	candidate.Config.Lite.Routes[0].CachePingTTL = configutil.Duration(time.Minute)
	require.True(t, g.ApplyLiveConfig(&candidate).Applied)

	candidate.Config.Lite.Routes[0].Host[0] = "mutated.example.test"
	candidate.Config.Lite.Routes[0].Backend[0] = "mutated.example.test:25565"
	candidate.Config.Lite.Routes[0].CachePingTTL = configutil.Duration(2 * time.Minute)

	published := g.Java().Config().Lite.Routes[0]
	require.Equal(t, []string{"play.example.test"}, []string(published.Host))
	require.Equal(t, []string{"backend.example.test:25565"}, []string(published.Backend))
	require.Equal(t, configutil.Duration(time.Minute), published.CachePingTTL)
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

func TestLoadLiveConfigCandidateRejectsInvalidFilesAndRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	v := viper.New()
	v.SetConfigFile(path)

	require.NoError(t, os.WriteFile(path, []byte("config:\n  lite: ["), 0o600))
	_, err := loadLiveConfigCandidate(v, path)
	require.Error(t, err)

	require.NoError(t, os.WriteFile(path, []byte("config:\n  unknownOption: true\n"), 0o600))
	_, err = loadLiveConfigCandidate(v, path)
	require.Error(t, err)

	require.NoError(t, os.WriteFile(path, []byte(`
config:
  lite:
    enabled: true
    routes:
      - host: play.example.test
        backend: backend.example.test:25565
        strategy: not-a-strategy
`), 0o600))
	semantic, err := loadLiveConfigCandidate(v, path)
	require.NoError(t, err)
	_, semanticErrors := semantic.Validate()
	require.NotEmpty(t, semanticErrors)

	require.NoError(t, os.WriteFile(path, []byte(`
config:
  lite:
    enabled: true
    routes:
      - host: play.example.test
        backend: backend.example.test:25565
        cachePingTTL: 45s
`), 0o600))
	recovered, err := loadLiveConfigCandidate(v, path)
	require.NoError(t, err)
	require.Equal(t, configutil.Duration(45*time.Second), recovered.Config.Lite.Routes[0].CachePingTTL)
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
