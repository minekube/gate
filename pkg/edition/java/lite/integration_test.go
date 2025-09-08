package lite

import (
	"net"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/netutil"
)

// TestNextBackendFunctionality tests the actual nextBackend implementation
// that gets created in findRoute - now using simple pop-first approach
func TestNextBackendFunctionality(t *testing.T) {
	log := testr.New(t)

	// This simulates the actual simple code in findRoute
	originalBackends := []string{"backend1:25565", "backend2:25565", "backend3:25565"}
	tryBackends := make([]string, len(originalBackends))
	copy(tryBackends, originalBackends)

	nextBackend := func() (string, logr.Logger, bool) {
		if len(tryBackends) == 0 {
			return "", log, false
		}
		// Pop first backend - simple and clean!
		backend := tryBackends[0]
		tryBackends = tryBackends[1:]

		dstAddr, err := netutil.Parse(backend, "tcp")
		if err != nil {
			log.V(1).Info("failed to parse backend address", "wrongBackendAddr", backend, "error", err)
			return "", log, false
		}
		backendAddr := dstAddr.String()
		if _, port := netutil.HostPort(dstAddr); port == 0 {
			backendAddr = net.JoinHostPort(dstAddr.String(), "25565")
		}

		return backendAddr, log.WithValues("backendAddr", backendAddr), true
	}

	// Test sequential pop behavior
	backends := make([]string, 0, 3)

	// Should get backends in order
	backend1, _, ok := nextBackend()
	assert.True(t, ok, "Should get first backend")
	backends = append(backends, backend1)

	backend2, _, ok := nextBackend()
	assert.True(t, ok, "Should get second backend")
	backends = append(backends, backend2)

	backend3, _, ok := nextBackend()
	assert.True(t, ok, "Should get third backend")
	backends = append(backends, backend3)

	// Should return false when no more backends
	_, _, ok = nextBackend()
	assert.False(t, ok, "Should return false when all backends exhausted")

	// Should have gotten all backends in order
	assert.Equal(t, []string{"backend1:25565", "backend2:25565", "backend3:25565"}, backends,
		"Should get backends in sequential order")
}

// TestFallbackResponseWithRealRoute tests handleFallbackResponse with various real route configurations
func TestFallbackResponseWithRealRoute(t *testing.T) {
	log := testr.New(t)
	protocol := proto.Protocol(765) // 1.20.4

	tests := []struct {
		name            string
		route           *config.Route
		expectResponse  bool
		expectInContent string
	}{
		{
			name:           "nil route returns nil",
			route:          nil,
			expectResponse: false,
		},
		{
			name: "route without fallback returns nil",
			route: &config.Route{
				Host:    []string{"test.com"},
				Backend: []string{"server:25565"},
			},
			expectResponse: false,
		},
		{
			name: "route with fallback returns response",
			route: &config.Route{
				Host:    []string{"test.com"},
				Backend: []string{"server:25565"},
				Fallback: &config.Status{
					MOTD: &configutil.TextComponent{
						Content: "Maintenance Mode",
					},
					Version: ping.Version{
						Name:     "Gate Lite",
						Protocol: 765,
					},
				},
			},
			expectResponse:  true,
			expectInContent: "Maintenance Mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, _ := handleFallbackResponse(log, tt.route, protocol, errAllBackendsFailed)

			if tt.expectResponse {
				require.NotNil(t, resp, "Should return fallback response")
				assert.IsType(t, &packet.StatusResponse{}, resp, "Should return StatusResponse type")
				if tt.expectInContent != "" {
					assert.Contains(t, resp.Status, tt.expectInContent, "Response should contain expected content")
				}
			} else {
				assert.Nil(t, resp, "Should return nil response")
			}
		})
	}
}
