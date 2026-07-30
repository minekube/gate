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
	"github.com/robinbraemer/event"
	"gopkg.in/yaml.v3"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/version"
)

// ConfigHandlerImpl implements the ConfigHandler interface.
type ConfigHandlerImpl struct {
	mu             *sync.Mutex
	cfg            *config.Config
	eventMgr       event.Manager
	proxy          *proxy.Proxy
	configFilePath string
}

func NewConfigHandler(mu *sync.Mutex, cfg *config.Config, eventMgr event.Manager, proxy *proxy.Proxy, configFilePath string) *ConfigHandlerImpl {
	return &ConfigHandlerImpl{
		mu:             mu,
		cfg:            cfg,
		eventMgr:       eventMgr,
		proxy:          proxy,
		configFilePath: configFilePath,
	}
}

func (h *ConfigHandlerImpl) GetStatus(context.Context, *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	h.mu.Lock()
	isLiteMode := h.cfg.Config.Lite.Enabled
	h.mu.Unlock()

	response := &pb.GetStatusResponse{Version: version.String()}
	if isLiteMode {
		response.Mode = pb.ProxyMode_PROXY_MODE_LITE

		h.mu.Lock()
		routes := h.cfg.Config.Lite.Routes
		h.mu.Unlock()

		var totalConnections int32
		if h.proxy != nil && h.proxy.Lite() != nil {
			sm := h.proxy.Lite().StrategyManager()
			for _, route := range routes {
				for _, backend := range route.Backend {
					if counter := sm.GetOrCreateCounter(backend); counter != nil {
						totalConnections += int32(counter.Load())
					}
				}
			}
		}
		response.Stats = &pb.GetStatusResponse_Lite{
			Lite: &pb.LiteStats{
				Connections: totalConnections,
				Routes:      int32(len(routes)),
			},
		}
		return response, nil
	}

	response.Mode = pb.ProxyMode_PROXY_MODE_CLASSIC
	var players, servers int32
	if h.proxy != nil {
		for _, server := range h.proxy.Servers() {
			server.Players().Range(func(proxy.Player) bool {
				players++
				return true
			})
		}
		servers = int32(len(h.proxy.Servers()))
	}
	response.Stats = &pb.GetStatusResponse_Classic{
		Classic: &pb.ClassicStats{
			Players: players,
			Servers: servers,
		},
	}
	return response, nil
}

func (h *ConfigHandlerImpl) GetConfig(context.Context, *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	h.mu.Lock()
	cfgCopy := *h.cfg
	h.mu.Unlock()

	data, err := yaml.Marshal(cfgCopy)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to encode config: %w", err))
	}
	return &pb.GetConfigResponse{Payload: string(data)}, nil
}

func (h *ConfigHandlerImpl) ValidateConfig(_ context.Context, req *pb.ValidateConfigRequest) ([]string, error) {
	var newCfg config.Config
	if err := yaml.Unmarshal([]byte(req.GetConfig()), &newCfg); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid YAML/JSON: %w", err))
	}
	warns, errs := newCfg.Validate()
	if len(errs) > 0 {
		errStrs := make([]string, len(errs))
		for i, err := range errs {
			errStrs[i] = err.Error()
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("config validation failed: %s", strings.Join(errStrs, "; ")))
	}
	warnStrs := make([]string, len(warns))
	for i, warning := range warns {
		warnStrs[i] = warning.Error()
	}
	return warnStrs, nil
}

func (h *ConfigHandlerImpl) ApplyConfig(ctx context.Context, req *pb.ApplyConfigRequest) ([]string, error) {
	var newCfg config.Config
	if err := yaml.Unmarshal([]byte(req.GetConfig()), &newCfg); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid YAML/JSON: %w", err))
	}

	warns, errs := newCfg.Validate()
	if len(errs) > 0 {
		errStrs := make([]string, len(errs))
		for i, err := range errs {
			errStrs[i] = err.Error()
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("config validation failed: %s", strings.Join(errStrs, "; ")))
	}

	h.mu.Lock()
	prev := *h.cfg
	*h.cfg = newCfg
	h.mu.Unlock()
	reload.FireConfigUpdate(h.eventMgr, h.cfg, &prev)
	logr.FromContextOrDiscard(ctx).Info("applied config via api")

	warnStrs := make([]string, len(warns))
	for i, warning := range warns {
		warnStrs[i] = warning.Error()
	}
	if req.GetPersist() {
		if err := h.persistConfig(&newCfg); err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "failed to persist config to disk (config applied in-memory)")
			warnStrs = append(warnStrs, fmt.Sprintf("failed to persist config to disk: %v", err))
		} else {
			logr.FromContextOrDiscard(ctx).Info("config persisted to disk")
		}
	}
	return warnStrs, nil
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
