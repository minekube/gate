package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A ".%s" usernameFormat without preserveDotPrefix is silently rewritten to
// "_%s" at login (see bedrock.javaCompatibleUsername). Operators then report the
// prefix as "overridden", so the rewrite has to be announced at validation time.
func TestBedrockDotPrefixRewriteWarning(t *testing.T) {
	const want = "bedrock.preserveDotPrefix"

	warnsFor := func(t *testing.T, mutate func(c *Config)) []string {
		t.Helper()
		cfg := DefaultConfig
		cfg.Forwarding.Mode = NoneForwardingMode
		cfg.Servers = map[string]string{"lobby": "127.0.0.1:25566"}
		cfg.Try = []string{"lobby"}
		mutate(&cfg)
		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		var out []string
		for _, w := range warns {
			out = append(out, w.Error())
		}
		return out
	}
	contains := func(warns []string) bool {
		for _, w := range warns {
			if strings.Contains(w, want) {
				return true
			}
		}
		return false
	}

	t.Run("dot format without opt-in warns", func(t *testing.T) {
		warns := warnsFor(t, func(c *Config) {
			c.Bedrock.Enabled = true
			c.Bedrock.UsernameFormat = ".%s"
		})
		require.True(t, contains(warns), "expected a warning mentioning %q, got %q", want, warns)
	})

	t.Run("dot format with opt-in is quiet", func(t *testing.T) {
		warns := warnsFor(t, func(c *Config) {
			c.Bedrock.Enabled = true
			c.Bedrock.UsernameFormat = ".%s"
			c.Bedrock.PreserveDotPrefix = true
		})
		require.False(t, contains(warns), "unexpected warning: %q", warns)
	})

	t.Run("default format is quiet", func(t *testing.T) {
		warns := warnsFor(t, func(c *Config) { c.Bedrock.Enabled = true })
		require.False(t, contains(warns), "unexpected warning: %q", warns)
	})

	t.Run("bedrock disabled is quiet", func(t *testing.T) {
		warns := warnsFor(t, func(c *Config) { c.Bedrock.UsernameFormat = ".%s" })
		require.False(t, contains(warns), "unexpected warning: %q", warns)
	})
}
