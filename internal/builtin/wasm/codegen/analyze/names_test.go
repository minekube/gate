package analyze

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWITIdentifier(t *testing.T) {
	tests := map[string]string{
		"HTTPServerID": "http-server-id",
		"URL2Path":     "url2-path",
		"snake_case":   "snake-case",
		"type":         "gate-type",
		"überValue":    "u00fc-ber-value",
		"":             "unnamed",
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, WITIdentifier(input))
		})
	}
}

func TestPackageWITNamesAreStableAndCollisionFree(t *testing.T) {
	paths := []string{
		"example.com/project/pkg/foo_bar",
		"example.com/project/pkg/foo-bar",
		"example.com/project/pkg/edition/java/proxy",
	}
	first, err := PackageWITNames("example.com/project", paths)
	require.NoError(t, err)
	second, err := PackageWITNames("example.com/project", []string{
		paths[2],
		paths[0],
		paths[1],
	})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, "pkg-edition-java-proxy", first[paths[2]])
	require.NotEqual(t, first[paths[0]], first[paths[1]])
	require.Contains(t, first[paths[0]], "pkg-foo-bar-")
	require.Contains(t, first[paths[1]], "pkg-foo-bar-")
}

func TestIdentityWITNamesAreStableAndCollisionFree(t *testing.T) {
	input := []NamedIdentity{
		{Identity: "example.Type.URL", GoName: "URL"},
		{Identity: "example.Type.Url", GoName: "Url"},
		{Identity: "example.Type.Name", GoName: "Name"},
	}
	names, err := IdentityWITNames(input)
	require.NoError(t, err)
	require.Equal(t, "name", names["example.Type.Name"])
	require.NotEqual(t, names["example.Type.URL"], names["example.Type.Url"])
	require.Contains(t, names["example.Type.URL"], "url-")
	require.Contains(t, names["example.Type.Url"], "url-")

	_, err = IdentityWITNames(append(input, input[0]))
	require.ErrorContains(t, err, "duplicate Go identity")
}
