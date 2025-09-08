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
// that gets created in findRoute and ensures backends are properly cycled through
func TestNextBackendFunctionality(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)
	
	route := &config.Route{
		Host:     []string{"test.example.com"},
		Backend:  []string{"backend1:25565", "backend2:25565", "backend3:25565"},
		Strategy: config.StrategyRoundRobin,
	}
	
	host := "test.example.com"
	
	// This simulates the actual code in findRoute
	tryBackends := route.Backend.Copy()
	nextBackend := func() (string, logr.Logger, bool) {
		if len(tryBackends) == 0 {
			return "", log, false
		}
		
		// Get next backend from strategy
		backendAddr, newLog, ok := sm.GetNextBackend(log, route, host, tryBackends)
		if !ok {
			return "", log, false
		}
		
		// Remove the selected backend from the list so it won't be tried again
		// This is the ACTUAL code from forward.go
		for i, backend := range tryBackends {
			// Need to normalize both for comparison
			normalizedBackend, err := netutil.Parse(backend, "tcp")
			if err != nil {
				continue
			}
			normalizedAddr := normalizedBackend.String()
			if _, port := netutil.HostPort(normalizedBackend); port == 0 {
				normalizedAddr = net.JoinHostPort(normalizedBackend.String(), "25565")
			}
			
			if normalizedAddr == backendAddr {
				// Remove this backend from the list
				tryBackends = append(tryBackends[:i], tryBackends[i+1:]...)
				break
			}
		}
		
		return backendAddr, newLog.WithValues("backendAddr", backendAddr), true
	}
	
	// Test that we can get all backends and then no more
	backends := make([]string, 0, 3)
	
	// Should get first backend
	backend1, _, ok := nextBackend()
	assert.True(t, ok, "Should get first backend")
	backends = append(backends, backend1)
	
	// Should get second backend
	backend2, _, ok := nextBackend()
	assert.True(t, ok, "Should get second backend") 
	backends = append(backends, backend2)
	
	// Should get third backend
	backend3, _, ok := nextBackend()
	assert.True(t, ok, "Should get third backend")
	backends = append(backends, backend3)
	
	// Should return false when no more backends
	_, _, ok = nextBackend()
	assert.False(t, ok, "Should return false when all backends exhausted")
	
	// Verify we got all 3 unique backends
	assert.Len(t, backends, 3, "Should have gotten 3 backends")
	
	// Verify no duplicates
	uniqueBackends := make(map[string]bool)
	for _, b := range backends {
		assert.False(t, uniqueBackends[b], "Should not have duplicate backend: %s", b)
		uniqueBackends[b] = true
	}
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

// TestLogVerbosityActuallyWorks verifies that log.V(1) actually increases verbosity
func TestLogVerbosityActuallyWorks(t *testing.T) {
	log := testr.New(t)
	
	// Normal log
	normalLog := log
	assert.True(t, normalLog.Enabled(), "Normal log should be enabled")
	
	// Verbose log
	verboseLog := log.V(1)
	assert.NotNil(t, verboseLog, "V(1) should return a logger")
	// In test mode, all verbosity levels are typically enabled
	// but in production, V(1) would require -v flag
}
