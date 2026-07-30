package api

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

type testConfigHandler struct {
	returnError bool
}

func (h *testConfigHandler) GetStatus(context.Context, *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return &pb.GetStatusResponse{Version: "test-version", Mode: pb.ProxyMode_PROXY_MODE_CLASSIC}, nil
}

func (h *testConfigHandler) GetConfig(context.Context, *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	if h.returnError {
		return nil, errors.New("test error")
	}
	return &pb.GetConfigResponse{Payload: "test: value", Version: "v1"}, nil
}

func (h *testConfigHandler) ValidateConfig(context.Context, *pb.ValidateConfigRequest) ([]string, error) {
	if h.returnError {
		return nil, errors.New("validation failed")
	}
	return []string{"warning"}, nil
}

func (h *testConfigHandler) ApplyConfig(context.Context, *pb.ApplyConfigRequest) (*pb.ApplyConfigResponse, error) {
	if h.returnError {
		return nil, errors.New("apply failed")
	}
	return &pb.ApplyConfigResponse{Warnings: []string{"applied"}, Version: "v2"}, nil
}

func TestServiceConfigMethodsDelegate(t *testing.T) {
	service := NewService(nil, &testConfigHandler{})

	status, err := service.GetStatus(context.Background(), connect.NewRequest(&pb.GetStatusRequest{}))
	require.NoError(t, err)
	require.Equal(t, "test-version", status.Msg.Version)

	cfg, err := service.GetConfig(context.Background(), connect.NewRequest(&pb.GetConfigRequest{}))
	require.NoError(t, err)
	require.Equal(t, "test: value", cfg.Msg.Payload)
	require.Equal(t, "v1", cfg.Msg.Version)

	validated, err := service.ValidateConfig(context.Background(), connect.NewRequest(&pb.ValidateConfigRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"warning"}, validated.Msg.Warnings)

	applied, err := service.ApplyConfig(context.Background(), connect.NewRequest(&pb.ApplyConfigRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"applied"}, applied.Msg.Warnings)
	require.Equal(t, "v2", applied.Msg.Version)
}

func TestServiceConfigMethodsRequireHandler(t *testing.T) {
	service := NewService(nil, nil)

	_, err := service.GetStatus(context.Background(), connect.NewRequest(&pb.GetStatusRequest{}))
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	_, err = service.GetConfig(context.Background(), connect.NewRequest(&pb.GetConfigRequest{}))
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	_, err = service.ValidateConfig(context.Background(), connect.NewRequest(&pb.ValidateConfigRequest{}))
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	_, err = service.ApplyConfig(context.Background(), connect.NewRequest(&pb.ApplyConfigRequest{}))
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
