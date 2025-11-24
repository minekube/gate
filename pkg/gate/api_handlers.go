package gate

import (
	"context"
	"encoding/json"
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

	config2 "go.minekube.com/gate/pkg/edition/java/lite/config"
	jping "go.minekube.com/gate/pkg/edition/java/ping"
	jver "go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
	gproto "go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/api"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/util/componentutil"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/version"
)

// ConfigHandlerImpl implements the ConfigHandler interface
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

func (h *ConfigHandlerImpl) GetStatus(ctx context.Context, req *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	h.mu.Lock()
	isLiteMode := h.cfg.Config.Lite.Enabled
	routes := h.cfg.Config.Lite.Routes
	h.mu.Unlock()

	response := &pb.GetStatusResponse{
		Version: version.String(),
	}

	if isLiteMode {
		response.Mode = pb.ProxyMode_PROXY_MODE_LITE

		// Get lite mode statistics
		h.mu.Lock()
		routes := h.cfg.Config.Lite.Routes
		h.mu.Unlock()

		// Count total active connections across all backends
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
	} else {
		response.Mode = pb.ProxyMode_PROXY_MODE_CLASSIC

		// Count players in classic mode
		var players int32
		var servers int32
		if h.proxy != nil {
			for _, s := range h.proxy.Servers() {
				s.Players().Range(func(proxy.Player) bool {
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
	}

	return response, nil
}

func (h *ConfigHandlerImpl) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	h.mu.Lock()
	cfgCopy := *h.cfg
	h.mu.Unlock()

	data, err := yaml.Marshal(cfgCopy)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to encode config: %w", err))
	}
	return &pb.GetConfigResponse{Payload: string(data)}, nil
}

func (h *ConfigHandlerImpl) ValidateConfig(ctx context.Context, req *pb.ValidateConfigRequest) ([]string, error) {
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
	for i, warn := range warns {
		warnStrs[i] = warn.Error()
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
	for i, warn := range warns {
		warnStrs[i] = warn.Error()
	}

	// If persist is enabled, try to write the config to disk
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
	configFile := h.configFilePath
	if configFile == "" {
		return errors.New("config file path not available - cannot persist config")
	}

	// Determine format from file extension
	var (
		data []byte
		err  error
	)
	switch path.Ext(configFile) {
	case ".yaml", ".yml":
		data, err = yaml.Marshal(cfg)
	default:
		return fmt.Errorf("unsupported config file format: %s (only .yml and .yaml are supported)", configFile)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", configFile, err)
	}

	return nil
}

// LiteHandlerImpl implements the LiteHandler interface
type LiteHandlerImpl struct {
	mu       *sync.Mutex
	cfg      *config.Config
	eventMgr event.Manager
	proxy    *proxy.Proxy
}

func NewLiteHandler(mu *sync.Mutex, cfg *config.Config, eventMgr event.Manager, proxy *proxy.Proxy) *LiteHandlerImpl {
	return &LiteHandlerImpl{
		mu:       mu,
		cfg:      cfg,
		eventMgr: eventMgr,
		proxy:    proxy,
	}
}

func (h *LiteHandlerImpl) ListLiteRoutes(ctx context.Context, req *pb.ListLiteRoutesRequest) (*pb.ListLiteRoutesResponse, error) {
	h.mu.Lock()
	routes := make([]config2.Route, len(h.cfg.Config.Lite.Routes))
	copy(routes, h.cfg.Config.Lite.Routes)
	h.mu.Unlock()
	resp := &pb.ListLiteRoutesResponse{}
	for _, r := range routes {
		resp.Routes = append(resp.Routes, h.toProtoRoute(r))
	}
	return resp, nil
}

func (h *LiteHandlerImpl) GetLiteRoute(ctx context.Context, req *pb.GetLiteRouteRequest) (*pb.GetLiteRouteResponse, error) {
	host := strings.TrimSpace(req.GetHost())
	if host == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host is required"))
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	idx := h.findRouteIdx(host)
	if idx < 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("route not found"))
	}
	return &pb.GetLiteRouteResponse{Route: h.toProtoRoute(h.cfg.Config.Lite.Routes[idx])}, nil
}

func (h *LiteHandlerImpl) UpdateLiteRouteStrategy(ctx context.Context, req *pb.UpdateLiteRouteStrategyRequest) ([]string, error) {
	host := strings.TrimSpace(req.GetHost())
	if host == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host is required"))
	}
	strategy := strings.TrimSpace(api.ConvertStrategyToString(req.GetStrategy()))
	if strategy == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("strategy is required"))
	}
	h.mu.Lock()
	newCfg := *h.cfg
	h.mu.Unlock()
	idx := h.findRouteIdxInConfig(&newCfg, host)
	if idx < 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("route not found"))
	}
	old := string(newCfg.Config.Lite.Routes[idx].Strategy)
	newCfg.Config.Lite.Routes[idx].Strategy = config2.Strategy(strategy)
	return h.applyConfigUpdate(ctx, newCfg, "lite route strategy updated", "host", host, "old", old, "new", strategy)
}

func (h *LiteHandlerImpl) AddLiteRouteBackend(ctx context.Context, req *pb.AddLiteRouteBackendRequest) ([]string, error) {
	host := strings.TrimSpace(req.GetHost())
	backend := strings.TrimSpace(req.GetBackend())
	if host == "" || backend == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host and backend are required"))
	}
	h.mu.Lock()
	newCfg := *h.cfg
	h.mu.Unlock()
	idx := h.findRouteIdxInConfig(&newCfg, host)
	if idx < 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("route not found"))
	}
	exists := false
	for _, b := range newCfg.Config.Lite.Routes[idx].Backend {
		if strings.EqualFold(b, backend) {
			exists = true
			break
		}
	}
	if !exists {
		newCfg.Config.Lite.Routes[idx].Backend = append(newCfg.Config.Lite.Routes[idx].Backend, backend)
	}
	return h.applyConfigUpdate(ctx, newCfg, "lite route backend added", "host", host, "backend", backend, "alreadyExisted", exists)
}

func (h *LiteHandlerImpl) RemoveLiteRouteBackend(ctx context.Context, req *pb.RemoveLiteRouteBackendRequest) ([]string, error) {
	host := strings.TrimSpace(req.GetHost())
	backend := strings.TrimSpace(req.GetBackend())
	if host == "" || backend == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host and backend are required"))
	}
	h.mu.Lock()
	newCfg := *h.cfg
	h.mu.Unlock()
	idx := h.findRouteIdxInConfig(&newCfg, host)
	if idx < 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("route not found"))
	}
	bs := newCfg.Config.Lite.Routes[idx].Backend
	filtered := make([]string, 0, len(bs))
	removed := false
	for _, b := range bs {
		if strings.EqualFold(b, backend) {
			removed = true
			continue
		}
		filtered = append(filtered, b)
	}
	newCfg.Config.Lite.Routes[idx].Backend = filtered
	return h.applyConfigUpdate(ctx, newCfg, "lite route backend removed", "host", host, "backend", backend, "removed", removed)
}

func (h *LiteHandlerImpl) UpdateLiteRouteOptions(ctx context.Context, req *pb.UpdateLiteRouteOptionsRequest) ([]string, error) {
	if req.GetOptions() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("options payload is required"))
	}
	host := strings.TrimSpace(req.GetHost())
	if host == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host is required"))
	}
	h.mu.Lock()
	newCfg := *h.cfg
	h.mu.Unlock()
	idx := h.findRouteIdxInConfig(&newCfg, host)
	if idx < 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("route not found"))
	}
	opts := req.GetOptions()
	paths := req.GetUpdateMask().GetPaths()
	if len(paths) == 0 {
		paths = []string{"proxy_protocol", "tcp_shield_real_ip", "modify_virtual_host", "cache_ping_ttl_ms"}
	}
	for _, path := range paths {
		switch path {
		case "proxy_protocol":
			newCfg.Config.Lite.Routes[idx].ProxyProtocol = opts.GetProxyProtocol()
		case "tcp_shield_real_ip":
			newCfg.Config.Lite.Routes[idx].TCPShieldRealIP = opts.GetTcpShieldRealIp()
		case "modify_virtual_host":
			newCfg.Config.Lite.Routes[idx].ModifyVirtualHost = opts.GetModifyVirtualHost()
		case "cache_ping_ttl_ms":
			newCfg.Config.Lite.Routes[idx].CachePingTTL = configutil.Duration(opts.GetCachePingTtlMs())
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported field mask path %q", path))
		}
	}
	return h.applyConfigUpdate(ctx, newCfg, "lite route options updated", "host", host)
}

func (h *LiteHandlerImpl) UpdateLiteRouteFallback(ctx context.Context, req *pb.UpdateLiteRouteFallbackRequest) ([]string, error) {
	host := strings.TrimSpace(req.GetHost())
	if host == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host is required"))
	}
	h.mu.Lock()
	newCfg := *h.cfg
	h.mu.Unlock()
	idx := h.findRouteIdxInConfig(&newCfg, host)
	if idx < 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("route not found"))
	}
	if newCfg.Config.Lite.Routes[idx].Fallback == nil {
		newCfg.Config.Lite.Routes[idx].Fallback = &config2.Status{}
	}
	fb := newCfg.Config.Lite.Routes[idx].Fallback
	paths := req.GetUpdateMask().GetPaths()
	if len(paths) == 0 {
		paths = []string{"motd_json", "version", "players", "favicon"}
	}
	for _, path := range paths {
		switch path {
		case "motd_json":
			if req.GetFallback() == nil || strings.TrimSpace(req.GetFallback().GetMotdJson()) == "" {
				fb.MOTD = nil
			} else {
				motd, err := h.parseMOTD(req.GetFallback().GetMotdJson())
				if err != nil {
					return nil, err
				}
				fb.MOTD = motd
			}
		case "version":
			if req.GetFallback() == nil || req.GetFallback().GetVersion() == nil {
				fb.Version = jping.Version{}
			} else {
				fb.Version.Name = req.GetFallback().GetVersion().GetName()
				fb.Version.Protocol = gproto.Protocol(req.GetFallback().GetVersion().GetProtocol())
			}
		case "players":
			if req.GetFallback() == nil || req.GetFallback().GetPlayers() == nil {
				fb.Players = nil
			} else {
				if fb.Players == nil {
					fb.Players = &jping.Players{}
				}
				fb.Players.Online = int(req.GetFallback().GetPlayers().GetOnline())
				fb.Players.Max = int(req.GetFallback().GetPlayers().GetMax())
			}
		case "favicon":
			if req.GetFallback() == nil || strings.TrimSpace(req.GetFallback().GetFavicon()) == "" {
				fb.Favicon = ""
			} else {
				fb.Favicon = favicon.Favicon(req.GetFallback().GetFavicon())
			}
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported field mask path %q", path))
		}
	}
	return h.applyConfigUpdate(ctx, newCfg, "lite route fallback updated", "host", host)
}

func (h *LiteHandlerImpl) toProtoFallback(src *config2.Status) *pb.LiteRouteFallback {
	if src == nil {
		return nil
	}
	pbFallback := &pb.LiteRouteFallback{}
	if src.MOTD != nil {
		if data, err := json.Marshal(src.MOTD); err == nil {
			pbFallback.MotdJson = string(data)
		}
	}
	if src.Version.Name != "" || src.Version.Protocol != 0 {
		pbFallback.Version = &pb.LiteRouteFallbackVersion{
			Name:     src.Version.Name,
			Protocol: int32(src.Version.Protocol),
		}
	}
	if src.Players != nil {
		pbFallback.Players = &pb.LiteRouteFallbackPlayers{
			Online: int32(src.Players.Online),
			Max:    int32(src.Players.Max),
		}
	}
	if src.Favicon != "" {
		pbFallback.Favicon = string(src.Favicon)
	}
	return pbFallback
}

func (h *LiteHandlerImpl) toProtoRoute(route config2.Route) *pb.LiteRoute {
	pbRoute := &pb.LiteRoute{
		Hosts:    route.Host,
		Strategy: api.ConvertStrategyFromString(string(route.Strategy)),
		Options: &pb.LiteRouteOptions{
			ProxyProtocol:     route.ProxyProtocol,
			TcpShieldRealIp:   route.GetTCPShieldRealIP(),
			ModifyVirtualHost: route.ModifyVirtualHost,
			CachePingTtlMs:    int64(route.CachePingTTL),
		},
	}

	for _, backend := range route.Backend {
		var active uint32
		if h.proxy != nil && h.proxy.Lite() != nil {
			sm := h.proxy.Lite().StrategyManager()
			if counter := sm.GetOrCreateCounter(backend); counter != nil {
				active = counter.Load()
			}
		}
		pbRoute.Backends = append(pbRoute.Backends, &pb.LiteRouteBackend{
			Address:           backend,
			ActiveConnections: active,
		})
	}
	pbRoute.Fallback = h.toProtoFallback(route.Fallback)
	return pbRoute
}

func (h *LiteHandlerImpl) findRouteIdx(host string) int {
	for i, r := range h.cfg.Config.Lite.Routes {
		for _, h := range r.Host {
			if strings.EqualFold(h, host) {
				return i
			}
		}
	}
	return -1
}

func (h *LiteHandlerImpl) findRouteIdxInConfig(c *config.Config, host string) int {
	for i, r := range c.Config.Lite.Routes {
		for _, h := range r.Host {
			if strings.EqualFold(h, host) {
				return i
			}
		}
	}
	return -1
}

func (h *LiteHandlerImpl) parseMOTD(s string) (*configutil.TextComponent, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	tc, err := componentutil.ParseTextComponent(jver.MinimumVersion.Protocol, s)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid motd: %w", err))
	}
	return (*configutil.TextComponent)(tc), nil
}

func (h *LiteHandlerImpl) applyConfigUpdate(ctx context.Context, newCfg config.Config, logMsg string, kv ...any) ([]string, error) {
	warns, errs := newCfg.Validate()
	if len(errs) > 0 {
		errStrs := make([]string, len(errs))
		for i, err := range errs {
			errStrs[i] = err.Error()
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("config validation failed: %s", strings.Join(errStrs, "; ")))
	}
	warnStrs := make([]string, len(warns))
	for i, warn := range warns {
		warnStrs[i] = warn.Error()
	}
	h.mu.Lock()
	prev := *h.cfg
	*h.cfg = newCfg
	h.mu.Unlock()
	reload.FireConfigUpdate(h.eventMgr, h.cfg, &prev)
	logr.FromContextOrDiscard(ctx).Info(logMsg, kv...)
	return warnStrs, nil
}