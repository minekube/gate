package gate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/version"
)

func TestVersionCommand(t *testing.T) {
	app := App()
	
	// Verify version is set correctly
	assert.Equal(t, version.String(), app.Version, "App version should match version package")
	
	// Verify version appears in help text
	help, err := app.ToMarkdown()
	require.NoError(t, err, "Should be able to generate help text")
	assert.Contains(t, help, "version", "Help should mention version command")
	
	// Test that our custom flags exist
	flags := make(map[string]bool)
	for _, flag := range app.Flags {
		flagNames := flag.Names()
		for _, name := range flagNames {
			if flags[name] {
				t.Errorf("Flag conflict detected: %s", name)
			}
			flags[name] = true
		}
	}
	
	// Verify our custom flags exist (version flags are added automatically by urfave/cli)
	assert.True(t, flags["verbosity"], "Verbosity flag should exist")
	assert.True(t, flags["verbose"], "Verbose alias should exist")
	assert.True(t, flags["config"], "Config flag should exist") 
	assert.True(t, flags["c"], "Config alias should exist")
	assert.True(t, flags["debug"], "Debug flag should exist")
	assert.True(t, flags["d"], "Debug alias should exist")
}

func TestVersionString(t *testing.T) {
	// Test that version string is accessible
	versionStr := version.String()
	require.NotEmpty(t, versionStr, "Version string should not be empty")
	
	// Should not panic or return empty in normal circumstances
	assert.True(t, len(versionStr) > 0, "Version should have content")
}

func TestUserAgentIncludesVersion(t *testing.T) {
	userAgent := version.UserAgent()
	
	// Should include Gate in user agent
	assert.Contains(t, userAgent, "Minekube-Gate", "User agent should include Minekube-Gate")
	
	// Should include version information (either actual version or "unknown")
	versionStr := version.String()
	if versionStr == "unknown" {
		assert.Contains(t, userAgent, "unknown", "User agent should include 'unknown' for unknown version")
	} else {
		assert.Contains(t, userAgent, versionStr, "User agent should include version string")
	}
}
