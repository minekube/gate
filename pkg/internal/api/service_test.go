package api

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

// Simple test handlers that return predictable responses
type testConfigHandler struct {
	returnError bool
}

func (h *testConfigHandler) GetStatus(ctx context.Context, req *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return &pb.GetStatusResponse{
		Version: "test-version",
		Mode:    pb.ProxyMode_PROXY_MODE_CLASSIC,
	}, nil
}

func (h *testConfigHandler) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return &pb.GetConfigResponse{Payload: "test: value"}, nil
}

func (h *testConfigHandler) ValidateConfig(ctx context.Context, req *pb.ValidateConfigRequest) ([]string, error) {
	if h.returnError {
		return nil, errors.New("validation failed")
	}
	return []string{"warning1", "warning2"}, nil
}

func (h *testConfigHandler) ApplyConfig(ctx context.Context, req *pb.ApplyConfigRequest) ([]string, error) {
	if h.returnError {
		return nil, errors.New("apply failed")
	}
	return []string{"applied"}, nil
}

type testLiteHandler struct {
	returnError bool
}

func (h *testLiteHandler) ListLiteRoutes(ctx context.Context, req *pb.ListLiteRoutesRequest) (*pb.ListLiteRoutesResponse, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return &pb.ListLiteRoutesResponse{
		Routes: []*pb.LiteRoute{
			{Hosts: []string{"test.com"}},
		},
	}, nil
}

func (h *testLiteHandler) GetLiteRoute(ctx context.Context, req *pb.GetLiteRouteRequest) (*pb.GetLiteRouteResponse, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return &pb.GetLiteRouteResponse{
		Route: &pb.LiteRoute{Hosts: []string{"test.com"}},
	}, nil
}

func (h *testLiteHandler) UpdateLiteRouteStrategy(ctx context.Context, req *pb.UpdateLiteRouteStrategyRequest) ([]string, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return []string{}, nil
}

func (h *testLiteHandler) AddLiteRouteBackend(ctx context.Context, req *pb.AddLiteRouteBackendRequest) ([]string, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return []string{}, nil
}

func (h *testLiteHandler) RemoveLiteRouteBackend(ctx context.Context, req *pb.RemoveLiteRouteBackendRequest) ([]string, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return []string{}, nil
}

func (h *testLiteHandler) UpdateLiteRouteOptions(ctx context.Context, req *pb.UpdateLiteRouteOptionsRequest) ([]string, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return []string{}, nil
}

func (h *testLiteHandler) UpdateLiteRouteFallback(ctx context.Context, req *pb.UpdateLiteRouteFallbackRequest) ([]string, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return []string{}, nil
}

func TestService_GetStatus(t *testing.T) {
	tests := []struct {
		name           string
		configHandler  ConfigHandler
		expectError    bool
		expectedStatus *pb.GetStatusResponse
	}{
		{
			name:          "success with config handler",
			configHandler: &testConfigHandler{returnError: false},
			expectError:   false,
			expectedStatus: &pb.GetStatusResponse{
				Version: "test-version",
				Mode:    pb.ProxyMode_PROXY_MODE_CLASSIC,
			},
		},
		{
			name:          "error when no config handler",
			configHandler: nil,
			expectError:   true,
		},
		{
			name:          "error from config handler",
			configHandler: &testConfigHandler{returnError: true},
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil, tt.configHandler, nil)

			resp, err := service.GetStatus(context.Background(), connect.NewRequest(&pb.GetStatusRequest{}))

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, tt.expectedStatus.Version, resp.Msg.Version)
				assert.Equal(t, tt.expectedStatus.Mode, resp.Msg.Mode)
			}
		})
	}
}

func TestService_GetConfig(t *testing.T) {
	tests := []struct {
		name          string
		configHandler ConfigHandler
		expectError   bool
		expectedYAML  string
	}{
		{
			name:          "success",
			configHandler: &testConfigHandler{returnError: false},
			expectError:   false,
			expectedYAML:  "test: value",
		},
		{
			name:          "no config handler",
			configHandler: nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil, tt.configHandler, nil)

			resp, err := service.GetConfig(context.Background(), connect.NewRequest(&pb.GetConfigRequest{}))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedYAML, resp.Msg.Payload)
			}
		})
	}
}

func TestService_ValidateConfig(t *testing.T) {
	tests := []struct {
		name             string
		configHandler    ConfigHandler
		expectError      bool
		expectedWarnings []string
	}{
		{
			name:             "success with warnings",
			configHandler:    &testConfigHandler{returnError: false},
			expectError:      false,
			expectedWarnings: []string{"warning1", "warning2"},
		},
		{
			name:          "validation error",
			configHandler: &testConfigHandler{returnError: true},
			expectError:   true,
		},
		{
			name:          "no config handler",
			configHandler: nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil, tt.configHandler, nil)

			resp, err := service.ValidateConfig(context.Background(), connect.NewRequest(&pb.ValidateConfigRequest{
				Config: "test: config",
			}))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedWarnings, resp.Msg.Warnings)
			}
		})
	}
}

func TestService_ApplyConfig(t *testing.T) {
	tests := []struct {
		name             string
		configHandler    ConfigHandler
		expectError      bool
		expectedWarnings []string
	}{
		{
			name:             "success",
			configHandler:    &testConfigHandler{returnError: false},
			expectError:      false,
			expectedWarnings: []string{"applied"},
		},
		{
			name:          "no config handler",
			configHandler: nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil, tt.configHandler, nil)

			resp, err := service.ApplyConfig(context.Background(), connect.NewRequest(&pb.ApplyConfigRequest{
				Config: "test: config",
			}))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedWarnings, resp.Msg.Warnings)
			}
		})
	}
}

func TestService_ListLiteRoutes(t *testing.T) {
	tests := []struct {
		name         string
		liteHandler  LiteHandler
		expectError  bool
		expectedLen  int
	}{
		{
			name:        "success",
			liteHandler: &testLiteHandler{returnError: false},
			expectError: false,
			expectedLen: 1,
		},
		{
			name:        "no lite handler",
			liteHandler: nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil, nil, tt.liteHandler)

			resp, err := service.ListLiteRoutes(context.Background(), connect.NewRequest(&pb.ListLiteRoutesRequest{}))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, resp.Msg.Routes, tt.expectedLen)
			}
		})
	}
}

func TestService_GetLiteRoute(t *testing.T) {
	tests := []struct {
		name        string
		liteHandler LiteHandler
		expectError bool
	}{
		{
			name:        "success",
			liteHandler: &testLiteHandler{returnError: false},
			expectError: false,
		},
		{
			name:        "no lite handler",
			liteHandler: nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil, nil, tt.liteHandler)

			resp, err := service.GetLiteRoute(context.Background(), connect.NewRequest(&pb.GetLiteRouteRequest{
				Host: "test.com",
			}))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []string{"test.com"}, resp.Msg.Route.Hosts)
			}
		})
	}
}

func TestService_UpdateLiteRouteStrategy(t *testing.T) {
	handler := &testLiteHandler{returnError: false}
	service := NewService(nil, nil, handler)

	resp, err := service.UpdateLiteRouteStrategy(context.Background(), connect.NewRequest(&pb.UpdateLiteRouteStrategyRequest{
		Host:     "test.com",
		Strategy: pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_ROUND_ROBIN,
	}))

	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.Warnings)
}

func TestService_AddLiteRouteBackend(t *testing.T) {
	handler := &testLiteHandler{returnError: false}
	service := NewService(nil, nil, handler)

	resp, err := service.AddLiteRouteBackend(context.Background(), connect.NewRequest(&pb.AddLiteRouteBackendRequest{
		Host:    "test.com",
		Backend: "server:25565",
	}))

	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.Warnings)
}

func TestService_RemoveLiteRouteBackend(t *testing.T) {
	handler := &testLiteHandler{returnError: false}
	service := NewService(nil, nil, handler)

	resp, err := service.RemoveLiteRouteBackend(context.Background(), connect.NewRequest(&pb.RemoveLiteRouteBackendRequest{
		Host:    "test.com",
		Backend: "server:25565",
	}))

	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.Warnings)
}

func TestService_UpdateLiteRouteOptions(t *testing.T) {
	handler := &testLiteHandler{returnError: false}
	service := NewService(nil, nil, handler)

	resp, err := service.UpdateLiteRouteOptions(context.Background(), connect.NewRequest(&pb.UpdateLiteRouteOptionsRequest{
		Host: "test.com",
		Options: &pb.LiteRouteOptions{
			ProxyProtocol: true,
		},
	}))

	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.Warnings)
}

func TestService_UpdateLiteRouteFallback(t *testing.T) {
	handler := &testLiteHandler{returnError: false}
	service := NewService(nil, nil, handler)

	resp, err := service.UpdateLiteRouteFallback(context.Background(), connect.NewRequest(&pb.UpdateLiteRouteFallbackRequest{
		Host: "test.com",
		Fallback: &pb.LiteRouteFallback{
			MotdJson: `{"text":"test"}`,
		},
	}))

	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.Warnings)
}