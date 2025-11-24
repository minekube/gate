package gate

import (
	"context"
	"sync"
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	config2 "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/gate/config"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

// Helper function to create a valid test config for lite mode
func createValidLiteTestConfig() *config.Config {
	cfg := &config.Config{}
	// Set required fields for validation
	cfg.Config.Bind = "localhost:25577"
	cfg.Config.Forwarding.Mode = "none"
	cfg.Config.Lite.Enabled = true
	cfg.Config.Lite.Routes = []config2.Route{
		{
			Host:     []string{"test1.com", "test1.net"},
			Backend:  []string{"server1:25565", "server2:25565"},
			Strategy: config2.StrategyRoundRobin,
		},
		{
			Host:     []string{"test2.com"},
			Backend:  []string{"server3:25565"},
			Strategy: config2.StrategySequential,
		},
	}
	return cfg
}

func TestLiteHandlerImpl_ListLiteRoutes(t *testing.T) {
	cfg := createValidLiteTestConfig()

	handler := NewLiteHandler(&sync.Mutex{}, cfg, event.Nop, nil)

	resp, err := handler.ListLiteRoutes(context.Background(), &pb.ListLiteRoutesRequest{})

	require.NoError(t, err)
	require.Len(t, resp.Routes, 2)

	// Check first route
	route1 := resp.Routes[0]
	assert.Equal(t, []string{"test1.com", "test1.net"}, route1.Hosts)
	assert.Len(t, route1.Backends, 2)
	assert.Equal(t, pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_ROUND_ROBIN, route1.Strategy)

	// Check second route
	route2 := resp.Routes[1]
	assert.Equal(t, []string{"test2.com"}, route2.Hosts)
	assert.Len(t, route2.Backends, 1)
	assert.Equal(t, pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL, route2.Strategy)
}

func TestLiteHandlerImpl_GetLiteRoute(t *testing.T) {
	cfg := createValidLiteTestConfig()
	// Add a specific route for this test
	cfg.Config.Lite.Routes = append(cfg.Config.Lite.Routes, config2.Route{
		Host:    []string{"test.com", "test.net"},
		Backend: []string{"server1:25565"},
	})

	handler := NewLiteHandler(&sync.Mutex{}, cfg, event.Nop, nil)

	tests := []struct {
		name        string
		host        string
		expectError bool
	}{
		{
			name:        "found route by exact host",
			host:        "test.com",
			expectError: false,
		},
		{
			name:        "found route by alternate host",
			host:        "test.net",
			expectError: false,
		},
		{
			name:        "case insensitive match",
			host:        "TEST.COM",
			expectError: false,
		},
		{
			name:        "route not found",
			host:        "notfound.com",
			expectError: true,
		},
		{
			name:        "empty host",
			host:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := handler.GetLiteRoute(context.Background(), &pb.GetLiteRouteRequest{
				Host: tt.host,
			})

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp.Route)
				assert.Contains(t, resp.Route.Hosts, "test.com")
				assert.Contains(t, resp.Route.Hosts, "test.net")
			}
		})
	}
}

func TestLiteHandlerImpl_UpdateLiteRouteStrategy(t *testing.T) {
	tests := []struct {
		name            string
		host            string
		strategy        pb.LiteRouteStrategy
		expectError     bool
		expectedInternalStrategy config2.Strategy
	}{
		{
			name:                     "update to round robin",
			host:                     "test.com",
			strategy:                 pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_ROUND_ROBIN,
			expectError:              false,
			expectedInternalStrategy: config2.StrategyRoundRobin,
		},
		{
			name:                     "update to sequential",
			host:                     "test.com",
			strategy:                 pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL,
			expectError:              false,
			expectedInternalStrategy: config2.StrategySequential,
		},
		{
			name:        "route not found",
			host:        "notfound.com",
			strategy:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_RANDOM,
			expectError: true,
		},
		{
			name:        "empty host",
			host:        "",
			strategy:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_RANDOM,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createValidLiteTestConfig()
			// Override with specific route for this test
			cfg.Config.Lite.Routes = []config2.Route{
				{
					Host:     []string{"test.com"},
					Backend:  []string{"server1:25565"},
					Strategy: config2.StrategySequential, // Start with sequential
				},
			}

			handler := NewLiteHandler(&sync.Mutex{}, cfg, event.Nop, nil)

			warnings, err := handler.UpdateLiteRouteStrategy(context.Background(), &pb.UpdateLiteRouteStrategyRequest{
				Host:     tt.host,
				Strategy: tt.strategy,
			})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, warnings) // Should return warnings slice

				// Verify the strategy was updated
				assert.Equal(t, tt.expectedInternalStrategy, cfg.Config.Lite.Routes[0].Strategy)
			}
		})
	}
}

func TestLiteHandlerImpl_AddLiteRouteBackend(t *testing.T) {
	tests := []struct {
		name            string
		host            string
		backend         string
		expectError     bool
		expectedBackends int
	}{
		{
			name:             "add new backend",
			host:             "test.com",
			backend:          "server2:25565",
			expectError:      false,
			expectedBackends: 2,
		},
		{
			name:             "add existing backend (case insensitive)",
			host:             "test.com",
			backend:          "SERVER1:25565",
			expectError:      false,
			expectedBackends: 1, // Should not duplicate
		},
		{
			name:        "route not found",
			host:        "notfound.com",
			backend:     "server2:25565",
			expectError: true,
		},
		{
			name:        "empty host",
			host:        "",
			backend:     "server2:25565",
			expectError: true,
		},
		{
			name:        "empty backend",
			host:        "test.com",
			backend:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createValidLiteTestConfig()
			// Override with specific route for this test
			cfg.Config.Lite.Routes = []config2.Route{
				{
					Host:    []string{"test.com"},
					Backend: []string{"server1:25565"},
				},
			}

			handler := NewLiteHandler(&sync.Mutex{}, cfg, event.Nop, nil)

			warnings, err := handler.AddLiteRouteBackend(context.Background(), &pb.AddLiteRouteBackendRequest{
				Host:    tt.host,
				Backend: tt.backend,
			})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, warnings)

				// Verify backend count
				assert.Len(t, cfg.Config.Lite.Routes[0].Backend, tt.expectedBackends)
			}
		})
	}
}

func TestLiteHandlerImpl_RemoveLiteRouteBackend(t *testing.T) {
	tests := []struct {
		name             string
		host             string
		backend          string
		expectError      bool
		expectedBackends int
	}{
		{
			name:             "remove existing backend",
			host:             "test.com",
			backend:          "server2:25565",
			expectError:      false,
			expectedBackends: 1,
		},
		{
			name:             "remove backend case insensitive",
			host:             "test.com",
			backend:          "SERVER1:25565",
			expectError:      false,
			expectedBackends: 1, // server2 remains
		},
		{
			name:             "remove non-existent backend",
			host:             "test.com",
			backend:          "server3:25565",
			expectError:      false,
			expectedBackends: 2, // No change
		},
		{
			name:        "route not found",
			host:        "notfound.com",
			backend:     "server1:25565",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createValidLiteTestConfig()
			// Override with specific route for this test
			cfg.Config.Lite.Routes = []config2.Route{
				{
					Host:    []string{"test.com"},
					Backend: []string{"server1:25565", "server2:25565"},
				},
			}

			handler := NewLiteHandler(&sync.Mutex{}, cfg, event.Nop, nil)

			warnings, err := handler.RemoveLiteRouteBackend(context.Background(), &pb.RemoveLiteRouteBackendRequest{
				Host:    tt.host,
				Backend: tt.backend,
			})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, warnings)

				// Verify backend count
				assert.Len(t, cfg.Config.Lite.Routes[0].Backend, tt.expectedBackends)
			}
		})
	}
}

func TestLiteHandlerImpl_UpdateLiteRouteOptions(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		options     *pb.LiteRouteOptions
		updateMask  *fieldmaskpb.FieldMask
		expectError bool
	}{
		{
			name: "update proxy protocol",
			host: "test.com",
			options: &pb.LiteRouteOptions{
				ProxyProtocol: true,
			},
			updateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"proxy_protocol"},
			},
			expectError: false,
		},
		{
			name: "update multiple options",
			host: "test.com",
			options: &pb.LiteRouteOptions{
				ProxyProtocol:     true,
				TcpShieldRealIp:   true,
				ModifyVirtualHost: false,
				CachePingTtlMs:    5000,
			},
			updateMask:  nil, // Should default to all fields
			expectError: false,
		},
		{
			name: "unsupported field mask path",
			host: "test.com",
			options: &pb.LiteRouteOptions{
				ProxyProtocol: true,
			},
			updateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"unsupported_field"},
			},
			expectError: true,
		},
		{
			name:        "nil options",
			host:        "test.com",
			options:     nil,
			expectError: true,
		},
		{
			name: "route not found",
			host: "notfound.com",
			options: &pb.LiteRouteOptions{
				ProxyProtocol: true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createValidLiteTestConfig()
			// Override with specific route for this test
			cfg.Config.Lite.Routes = []config2.Route{
				{
					Host:    []string{"test.com"},
					Backend: []string{"server1:25565"},
				},
			}

			handler := NewLiteHandler(&sync.Mutex{}, cfg, event.Nop, nil)

			warnings, err := handler.UpdateLiteRouteOptions(context.Background(), &pb.UpdateLiteRouteOptionsRequest{
				Host:       tt.host,
				Options:    tt.options,
				UpdateMask: tt.updateMask,
			})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, warnings)

				// Verify options were updated
				route := &cfg.Config.Lite.Routes[0]
				if tt.updateMask == nil || contains(tt.updateMask.Paths, "proxy_protocol") {
					assert.Equal(t, tt.options.ProxyProtocol, route.ProxyProtocol)
				}
			}
		})
	}
}

func TestLiteHandlerImpl_UpdateLiteRouteFallback(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		fallback    *pb.LiteRouteFallback
		updateMask  *fieldmaskpb.FieldMask
		expectError bool
	}{
		{
			name: "update MOTD",
			host: "test.com",
			fallback: &pb.LiteRouteFallback{
				MotdJson: `{"text":"Maintenance Mode"}`,
			},
			updateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"motd_json"},
			},
			expectError: false,
		},
		{
			name: "update version info",
			host: "test.com",
			fallback: &pb.LiteRouteFallback{
				Version: &pb.LiteRouteFallbackVersion{
					Name:     "Gate Lite 1.0",
					Protocol: 765,
				},
			},
			updateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"version"},
			},
			expectError: false,
		},
		{
			name: "update players",
			host: "test.com",
			fallback: &pb.LiteRouteFallback{
				Players: &pb.LiteRouteFallbackPlayers{
					Online: 50,
					Max:    100,
				},
			},
			updateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"players"},
			},
			expectError: false,
		},
		{
			name: "clear MOTD with empty string",
			host: "test.com",
			fallback: &pb.LiteRouteFallback{
				MotdJson: "",
			},
			updateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"motd_json"},
			},
			expectError: false,
		},
		{
			name: "invalid MOTD JSON",
			host: "test.com",
			fallback: &pb.LiteRouteFallback{
				MotdJson: `{"text":"unclosed`,
			},
			updateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"motd_json"},
			},
			expectError: true,
		},
		{
			name: "route not found",
			host: "notfound.com",
			fallback: &pb.LiteRouteFallback{
				MotdJson: `{"text":"test"}`,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createValidLiteTestConfig()
			// Override with specific route for this test
			cfg.Config.Lite.Routes = []config2.Route{
				{
					Host:    []string{"test.com"},
					Backend: []string{"server1:25565"},
				},
			}

			handler := NewLiteHandler(&sync.Mutex{}, cfg, event.Nop, nil)

			warnings, err := handler.UpdateLiteRouteFallback(context.Background(), &pb.UpdateLiteRouteFallbackRequest{
				Host:       tt.host,
				Fallback:   tt.fallback,
				UpdateMask: tt.updateMask,
			})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, warnings)

				// Verify fallback was created/updated
				route := &cfg.Config.Lite.Routes[0]
				if route.Fallback != nil {
					if tt.updateMask == nil || contains(tt.updateMask.Paths, "motd_json") {
						if tt.fallback.MotdJson == "" {
							assert.Nil(t, route.Fallback.MOTD)
						} else if tt.fallback.MotdJson != "" {
							assert.NotNil(t, route.Fallback.MOTD)
						}
					}
				}
			}
		})
	}
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}