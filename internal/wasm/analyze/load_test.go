package analyze

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDiscoversOnlyPublicScopedDeclarations(t *testing.T) {
	result, err := Load(context.Background(), Options{
		Dir:        filepath.Join("testdata", "module"),
		ModulePath: "example.com/fixture",
		Patterns:   []string{"./api/...", "./pkg/..."},
		Exclusions: []Exclusion{{
			Identity: "example.com/fixture/pkg/public.Excluded",
			Reason:   "fixture exclusion",
		}},
	})
	require.NoError(t, err)

	require.Equal(t, []string{
		"example.com/fixture/api/service",
		"example.com/fixture/pkg/public",
	}, result.Packages)

	identities := declarationIdentities(result.Declarations)
	require.Equal(t, []string{
		"example.com/fixture/api/service.APIConstant",
		"example.com/fixture/api/service.APIFunction",
		"example.com/fixture/api/service.APIStruct",
		"example.com/fixture/api/service.APIVariable",
		"example.com/fixture/pkg/public.Alias",
		"example.com/fixture/pkg/public.Constant",
		"example.com/fixture/pkg/public.Exported",
		"example.com/fixture/pkg/public.Exported.PointerMethod",
		"example.com/fixture/pkg/public.Exported.ValueMethod",
		"example.com/fixture/pkg/public.Function",
		"example.com/fixture/pkg/public.Variable",
	}, identities)
	require.Equal(t, []ExcludedDeclaration{{
		Declaration: Declaration{
			Identity:    "example.com/fixture/pkg/public.Excluded",
			PackagePath: "example.com/fixture/pkg/public",
			Name:        "Excluded",
			Kind:        DeclarationType,
		},
		Reason: "fixture exclusion",
	}}, result.Excluded)

	for _, identity := range append(identities, excludedIdentities(result.Excluded)...) {
		require.NotContains(t, identity, "/internal/")
		require.NotContains(t, identity, "/cmd/")
		require.NotContains(t, identity, "unexported")
		require.NotContains(t, identity, "TestOnly")
	}
}

func TestLoadRejectsStaleAndInvalidExclusions(t *testing.T) {
	tests := []struct {
		name      string
		exclusion Exclusion
		want      string
	}{
		{
			name: "stale identity",
			exclusion: Exclusion{
				Identity: "example.com/fixture/pkg/public.Missing",
				Reason:   "does not exist",
			},
			want: "did not match a public declaration",
		},
		{
			name: "missing reason",
			exclusion: Exclusion{
				Identity: "example.com/fixture/pkg/public.Excluded",
			},
			want: "must include a reason",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(context.Background(), Options{
				Dir:        filepath.Join("testdata", "module"),
				ModulePath: "example.com/fixture",
				Patterns:   []string{"./api/...", "./pkg/..."},
				Exclusions: []Exclusion{tt.exclusion},
			})
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestLoadGateRepositoryScopeAndBootstrapPolicy(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)

	result, err := Load(context.Background(), GateOptions(root))
	require.NoError(t, err)
	require.NotEmpty(t, result.Declarations)

	identities := declarationIdentities(result.Declarations)
	require.True(t, slices.Contains(
		identities,
		"go.minekube.com/gate/pkg/edition/java/proxy.Proxy",
	))
	require.True(t, slices.Contains(
		identities,
		"go.minekube.com/gate/pkg/edition/java/proxy.Proxy.Players",
	))
	for _, pkg := range result.Packages {
		require.False(t, strings.Contains(pkg, "/internal/"), pkg)
		require.False(t, strings.HasPrefix(pkg, "go.minekube.com/gate/cmd/"), pkg)
	}
	require.Equal(t, []string{
		"go.minekube.com/gate/pkg/edition/java/proxy.Plugin",
		"go.minekube.com/gate/pkg/edition/java/proxy.Plugins",
	}, excludedIdentities(result.Excluded))
}

func declarationIdentities(declarations []Declaration) []string {
	identities := make([]string, len(declarations))
	for i, declaration := range declarations {
		identities[i] = declaration.Identity
	}
	return identities
}

func excludedIdentities(declarations []ExcludedDeclaration) []string {
	identities := make([]string, len(declarations))
	for i, declaration := range declarations {
		identities[i] = declaration.Declaration.Identity
	}
	return identities
}
