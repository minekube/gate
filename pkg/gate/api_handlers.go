package gate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
	"go.minekube.com/gate/pkg/version"
)

// ConfigHandlerImpl exposes the effective Gate configuration through the
// transactional live-reload path.
type ConfigHandlerImpl struct {
	applyMu        sync.Mutex
	gate           *Gate
	configFilePath string
}

func NewConfigHandler(gate *Gate, configFilePath string) *ConfigHandlerImpl {
	return &ConfigHandlerImpl{gate: gate, configFilePath: configFilePath}
}

func (h *ConfigHandlerImpl) GetStatus(context.Context, *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	cfg, _, err := h.gate.ConfigSnapshot()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("effective config is unavailable"))
	}

	response := &pb.GetStatusResponse{Version: version.String()}
	if cfg.Config.Lite.Enabled {
		response.Mode = pb.ProxyMode_PROXY_MODE_LITE
		var totalConnections int32
		if p := h.gate.Java(); p != nil && p.Lite() != nil {
			totalConnections = int32(p.Lite().StrategyManager().ActiveConnections())
		}
		response.Stats = &pb.GetStatusResponse_Lite{
			Lite: &pb.LiteStats{
				Connections: totalConnections,
				Routes:      int32(len(cfg.Config.Lite.Routes)),
			},
		}
		return response, nil
	}

	response.Mode = pb.ProxyMode_PROXY_MODE_CLASSIC
	var players, servers int32
	if p := h.gate.Java(); p != nil {
		for _, server := range p.Servers() {
			server.Players().Range(func(proxy.Player) bool {
				players++
				return true
			})
		}
		servers = int32(len(p.Servers()))
	}
	response.Stats = &pb.GetStatusResponse_Classic{
		Classic: &pb.ClassicStats{Players: players, Servers: servers},
	}
	return response, nil
}

func (h *ConfigHandlerImpl) GetConfig(context.Context, *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	cfg, configVersion, err := h.gate.ConfigSnapshot()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("effective config is unavailable"))
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encode effective config"))
	}
	return &pb.GetConfigResponse{Payload: string(data), Version: configVersion}, nil
}

func (h *ConfigHandlerImpl) ValidateConfig(_ context.Context, req *pb.ValidateConfigRequest) ([]string, error) {
	candidate, err := decodeAPIConfig(req.GetConfig())
	if err != nil {
		return nil, err
	}
	return validateAPIConfig(candidate)
}

func (h *ConfigHandlerImpl) ApplyConfig(ctx context.Context, req *pb.ApplyConfigRequest) (*pb.ApplyConfigResponse, error) {
	h.applyMu.Lock()
	defer h.applyMu.Unlock()

	if req.GetIfMatch() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("if_match is required"))
	}
	current, _, err := h.gate.ConfigSnapshot()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("effective config is unavailable"))
	}

	var candidate *config.Config
	switch input := req.GetInput().(type) {
	case *pb.ApplyConfigRequest_Config:
		candidate, err = decodeAPIConfig(input.Config)
	case *pb.ApplyConfigRequest_MergePatch:
		candidate, err = mergeConfigPatch(current, input.MergePatch)
	default:
		err = errors.New("config or merge_patch is required")
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	warnings, err := validateAPIConfig(candidate)
	if err != nil {
		return nil, err
	}
	result := h.gate.ApplyLiveConfigIfVersion(candidate, req.GetIfMatch())
	switch result.Code {
	case "applied", "unchanged":
	case "precondition_failed":
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("configuration version does not match"))
	case "unsupported":
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("configuration contains restart-required changes; only Java Lite routes can be reloaded"))
	case "invalid":
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("configuration is invalid"))
	default:
		return nil, connect.NewError(connect.CodeInternal, errors.New("configuration could not be applied"))
	}

	if req.GetPersist() {
		if err := h.persistConfig(candidate); err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "failed to persist config to disk (config applied in-memory)")
			warnings = append(warnings, fmt.Sprintf("failed to persist config to disk: %v", err))
		} else {
			logr.FromContextOrDiscard(ctx).Info("config persisted to disk")
		}
	}
	logr.FromContextOrDiscard(ctx).Info("applied config via api")
	return &pb.ApplyConfigResponse{Warnings: warnings, Version: result.Version}, nil
}

func decodeAPIConfig(payload string) (*config.Config, error) {
	var candidate config.Config
	if err := decodeConfigStrict([]byte(payload), ".yaml", &candidate); err != nil {
		return nil, fmt.Errorf("invalid YAML/JSON: %w", err)
	}
	return &candidate, nil
}

func validateAPIConfig(candidate *config.Config) ([]string, error) {
	warns, errs := candidate.Validate()
	if len(errs) > 0 {
		errStrs := make([]string, len(errs))
		for i, err := range errs {
			errStrs[i] = err.Error()
		}
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("config validation failed: %s", strings.Join(errStrs, "; ")))
	}
	warnings := make([]string, len(warns))
	for i, warning := range warns {
		warnings[i] = warning.Error()
	}
	return warnings, nil
}

func (h *ConfigHandlerImpl) persistConfig(cfg *config.Config) error {
	if h.configFilePath == "" {
		return errors.New("config file path not available - cannot persist config")
	}
	if ext := path.Ext(h.configFilePath); ext != ".yaml" && ext != ".yml" {
		return fmt.Errorf("unsupported config file format: %s (only .yml and .yaml are supported)", h.configFilePath)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(h.configFilePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", h.configFilePath, err)
	}
	return nil
}
