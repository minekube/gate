package gate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"go.minekube.com/gate/pkg/gate/config"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

func TestConfigHandlerRequiresVersionAndAppliesMergePatch(t *testing.T) {
	g, err := New(Options{Config: liveReloadConfig()})
	require.NoError(t, err)
	handler := NewConfigHandler(g, "")

	current, err := handler.GetConfig(context.Background(), &pb.GetConfigRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, current.Payload)
	require.NotEmpty(t, current.Version)

	patch := &pb.ApplyConfigRequest{
		Input: &pb.ApplyConfigRequest_MergePatch{MergePatch: `{
			"config": {
				"lite": {
					"routes": [{
						"host": "patched.example.test",
						"backend": "patched-backend.example.test:25565"
					}]
				}
			}
		}`},
	}
	_, err = handler.ApplyConfig(context.Background(), patch)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	patch.IfMatch = "stale"
	_, err = handler.ApplyConfig(context.Background(), patch)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	patch.IfMatch = current.Version
	applied, err := handler.ApplyConfig(context.Background(), patch)
	require.NoError(t, err)
	require.NotEqual(t, current.Version, applied.Version)

	snapshot, version, err := g.ConfigSnapshot()
	require.NoError(t, err)
	require.Equal(t, applied.Version, version)
	require.Equal(t, []string{"patched.example.test"}, []string(snapshot.Config.Lite.Routes[0].Host))
}

func TestConfigHandlerRejectsRestartRequiredChange(t *testing.T) {
	g, err := New(Options{Config: liveReloadConfig()})
	require.NoError(t, err)
	handler := NewConfigHandler(g, "")

	current, err := handler.GetConfig(context.Background(), &pb.GetConfigRequest{})
	require.NoError(t, err)
	_, err = handler.ApplyConfig(context.Background(), &pb.ApplyConfigRequest{
		Input:   &pb.ApplyConfigRequest_MergePatch{MergePatch: `{"config":{"bind":"127.0.0.1:25566"}}`},
		IfMatch: current.Version,
	})
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	after, err := handler.GetConfig(context.Background(), &pb.GetConfigRequest{})
	require.NoError(t, err)
	require.Equal(t, current.Version, after.Version)
}

func TestConfigHandlerPersistsAppliedSnapshot(t *testing.T) {
	g, err := New(Options{Config: liveReloadConfig()})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	handler := NewConfigHandler(g, path)

	current, err := handler.GetConfig(context.Background(), &pb.GetConfigRequest{})
	require.NoError(t, err)
	applied, err := handler.ApplyConfig(context.Background(), &pb.ApplyConfigRequest{
		Input: &pb.ApplyConfigRequest_MergePatch{MergePatch: `{
			"config": {
				"lite": {
					"routes": [{
						"host": "persisted.example.test",
						"backend": "backend.example.test:25565"
					}]
				}
			}
		}`},
		IfMatch: current.Version,
		Persist: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, applied.Version)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	var persisted config.Config
	require.NoError(t, yaml.Unmarshal(content, &persisted))
	require.Equal(t, []string{"persisted.example.test"}, []string(persisted.Config.Lite.Routes[0].Host))
}
