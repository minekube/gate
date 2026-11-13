package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/util/configutil"
)

// Regression coverage for the netutil port-range fix at the validation seam
// (CodeQL alert #1: "Incorrect conversion between integer types" in
// pkg/util/netutil). A Lite backend whose port is out of range used to be
// accepted silently - the port wrapped into a different one on the way through
// netutil (70000 -> 4464, -1 -> 65535), so the config validated and the route
// ended up with a port that was never written in the config.
func TestValidate_rejectsOutOfRangeBackendPort(t *testing.T) {
	for _, addr := range []string{"backend:70000", "backend:65536", "backend:-1"} {
		t.Run(addr, func(t *testing.T) {
			c := Config{Routes: []Route{{
				Host:    configutil.SingleOrMulti[string]{"*"},
				Backend: configutil.SingleOrMulti[string]{addr},
			}}}

			warns, errs := c.Validate()

			require.Empty(t, warns)
			require.Len(t, errs, 1, "an out-of-range backend port must fail validation")
			require.ErrorContains(t, errs[0], "failed to parse address")
			require.ErrorContains(t, errs[0], "out of range")
		})
	}
}

func TestValidate_acceptsInRangeBackendPort(t *testing.T) {
	for _, addr := range []string{"backend:25565", "backend:0", "backend:65535"} {
		t.Run(addr, func(t *testing.T) {
			c := Config{Routes: []Route{{
				Host:    configutil.SingleOrMulti[string]{"*"},
				Backend: configutil.SingleOrMulti[string]{addr},
			}}}

			warns, errs := c.Validate()

			require.Empty(t, warns)
			require.Empty(t, errs)
		})
	}
}
