package gate

import (
	"bytes"
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
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	config2 "go.minekube.com/gate/pkg/edition/java/lite/config"
	jping "go.minekube.com/gate/pkg/edition/java/ping"
	jver "go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
	gproto "go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/api"
	apipb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
	"go.minekube.com/gate/pkg/internal/hashutil"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/runtime/process"
	"go.minekube.com/gate/pkg/util/componentutil"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/version"
)

func setupAPI(cfg *config.Config, eventMgr event.Manager, initialEnable *proxy.Proxy) process.Runnable {
	return process.RunnableFunc(func(ctx context.Context) error {
		log := logr.FromContextOrDiscard(ctx).WithName("api")
		ctx = logr.NewContext(ctx, log)

		var (
			mu                sync.Mutex
			stop              context.CancelFunc
			currentConfigHash []byte
		)
		trigger := func(c *reload.ConfigUpdateEvent[config.Config]) {
			newConfigHash, err := hashutil.JsonHash(c.Config.API)
			if err != nil {
				log.Error(err, "error hashing API config")
				return
			}

			mu.Lock()
			defer mu.Unlock()

			// check if config changed
			if bytes.Equal(newConfigHash, currentConfigHash) {
				return // no change
			}
			currentConfigHash = newConfigHash

			if stop != nil {
				stop()
				stop = nil
			}

			if c.Config.API.Enabled {
				errorsToStrings := func(errs []error) []string {
					if len(errs) == 0 {
						return nil
					}
					out := make([]string, 0, len(errs))
					for _, err := range errs {
						out = append(out, err.Error())
					}
					return out
				}

				validationError := func(msg string, errs []error) error {
					if len(errs) == 0 {
						return nil
					}
					return connect.NewError(connect.CodeInvalidArgument, errors.New(msg+": "+strings.Join(errorsToStrings(errs), "; ")))
				}

				convertProtoToGoConfig := func(protoConfig *apipb.GateConfig) (config.Config, error) {
					// Convert protobuf to JSON
					data, err := json.Marshal(protoConfig)
					if err != nil {
						return config.Config{}, fmt.Errorf("failed to marshal protobuf config: %w", err)
					}

					// Parse JSON to map for transformation
					var configMap map[string]interface{}
					if err := json.Unmarshal(data, &configMap); err != nil {
						return config.Config{}, fmt.Errorf("failed to unmarshal JSON to map: %w", err)
					}

					// Transform API config structure to match Go struct expectations
					if apiVal, exists := configMap["api"]; exists {
						if apiMap, ok := apiVal.(map[string]interface{}); ok {
							if bindVal, hasBindVal := apiMap["bind"]; hasBindVal {
								// Create nested Config structure that Go expects
								apiMap["Config"] = map[string]interface{}{
									"bind": bindVal,
								}
								// Remove the direct bind field
								delete(apiMap, "bind")
							}
						}
					}

					// Convert back to JSON and unmarshal to Go struct
					transformedData, err := json.Marshal(configMap)
					if err != nil {
						return config.Config{}, fmt.Errorf("failed to marshal transformed config: %w", err)
					}


					var goConfig config.Config
					if err := json.Unmarshal(transformedData, &goConfig); err != nil {
						return config.Config{}, fmt.Errorf("failed to unmarshal to Go config: %w", err)
					}

					return goConfig, nil
				}

				decodeConfigOneof := func(yamlConfig string, jsonConfig *apipb.GateConfig) (config.Config, error) {
					if yamlConfig != "" && jsonConfig != nil {
						return config.Config{}, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot specify both yaml_config and json_config"))
					}
					if yamlConfig == "" && jsonConfig == nil {
						return config.Config{}, connect.NewError(connect.CodeInvalidArgument, errors.New("must specify either yaml_config or json_config"))
					}

					var newCfg config.Config
					var err error

					if yamlConfig != "" {
						// Handle YAML config
						if err := yaml.Unmarshal([]byte(yamlConfig), &newCfg); err != nil {
							return config.Config{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid YAML: %w", err))
						}
					} else if jsonConfig != nil {
						// Convert protobuf GateConfig to Go config.Config
						newCfg, err = convertProtoToGoConfig(jsonConfig)
						if err != nil {
							return config.Config{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("failed to convert config: %w", err))
						}
					}

					return newCfg, nil
				}

				redactConfig := func(src *config.Config) config.Config {
					mu.Lock()
					defer mu.Unlock()
					copy := *src
					copy.Config.Forwarding.VelocitySecret = ""
					copy.Config.Forwarding.BungeeGuardSecret = ""
					return copy
				}

				applyConfigUpdate := func(ctx context.Context, newCfg config.Config, logMsg string, kv ...any) ([]string, error) {
					warns, errs := newCfg.Validate()
					if err := validationError("config validation failed", errs); err != nil {
						return nil, err
					}
					warnMessages := errorsToStrings(warns)
					mu.Lock()
					prev := *cfg
					*cfg = newCfg
					mu.Unlock()
					reload.FireConfigUpdate(eventMgr, cfg, &prev)
					logr.FromContextOrDiscard(ctx).Info(logMsg, kv...)
					return warnMessages, nil
				}

				persistConfig := func(cfg *config.Config) error {
					// Try to determine config file path using Viper
					v := viper.New()
					v.SetConfigName("config")
					v.SetConfigType("yaml") // Default to YAML
					v.AddConfigPath(".")

					// Try to find existing config file
					if err := v.ReadInConfig(); err != nil {
						return fmt.Errorf("no existing config file found to overwrite: %w", err)
					}

					configFile := v.ConfigFileUsed()
					if configFile == "" {
						return errors.New("could not determine config file path")
					}

					// Determine format from file extension
					var (
						data []byte
						err  error
					)
					switch path.Ext(configFile) {
					case ".yaml", ".yml":
						data, err = yaml.Marshal(cfg)
					case ".json":
						data, err = json.MarshalIndent(cfg, "", "  ")
					default:
						return fmt.Errorf("unsupported config file format: %s", configFile)
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

				toProtoFallback := func(src *config2.Status) *apipb.LiteRouteFallback {
					if src == nil {
						return nil
					}
					pbFallback := &apipb.LiteRouteFallback{}
					if src.MOTD != nil {
						if data, err := json.Marshal(src.MOTD); err == nil {
							pbFallback.MotdJson = string(data)
						}
					}
					if src.Version.Name != "" || src.Version.Protocol != 0 {
						pbFallback.Version = &apipb.LiteRouteFallbackVersion{
							Name:     src.Version.Name,
							Protocol: int32(src.Version.Protocol),
						}
					}
					if src.Players != nil {
						pbFallback.Players = &apipb.LiteRouteFallbackPlayers{
							Online: int32(src.Players.Online),
							Max:    int32(src.Players.Max),
						}
					}
					if src.Favicon != "" {
						pbFallback.Favicon = string(src.Favicon)
					}
					return pbFallback
				}

				toProtoRoute := func(route config2.Route) *apipb.LiteRoute {
					sm := initialEnable.Lite().StrategyManager()
					pbRoute := &apipb.LiteRoute{
						Hosts:    route.Host,
						Strategy: string(route.Strategy),
						Options: &apipb.LiteRouteOptions{
							ProxyProtocol:     route.ProxyProtocol,
							TcpShieldRealIp:   route.GetTCPShieldRealIP(),
							ModifyVirtualHost: route.ModifyVirtualHost,
							CachePingTtlMs:    int64(route.CachePingTTL),
						},
					}
					for _, backend := range route.Backend {
						var active uint32
						if counter := sm.GetOrCreateCounter(backend); counter != nil {
							active = counter.Load()
						}
						pbRoute.Backends = append(pbRoute.Backends, &apipb.LiteRouteBackend{
							Address:           backend,
							ActiveConnections: active,
						})
					}
					pbRoute.Fallback = toProtoFallback(route.Fallback)
					return pbRoute
				}

				findRouteIdx := func(c *config.Config, host string) int {
					for i, r := range c.Config.Lite.Routes {
						for _, h := range r.Host {
							if strings.EqualFold(h, host) {
								return i
							}
						}
					}
					return -1
				}

				parseMOTD := func(s string) (*configutil.TextComponent, error) {
					if strings.TrimSpace(s) == "" {
						return nil, nil
					}
					tc, err := componentutil.ParseTextComponent(jver.MinimumVersion.Protocol, s)
					if err != nil {
						return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid motd: %w", err))
					}
					return (*configutil.TextComponent)(tc), nil
				}

				handlers := api.ServiceHandlers{
					GetStatus: func(ctx context.Context, _ *apipb.GetStatusRequest) (*apipb.GetStatusResponse, error) {
						statusCfg := redactConfig(cfg)
						players := 0
						for _, s := range initialEnable.Servers() {
							s.Players().Range(func(proxy.Player) bool {
								players++
								return true
							})
						}
						mode := apipb.ProxyMode_PROXY_MODE_CLASSIC
						if statusCfg.Config.Lite.Enabled {
							mode = apipb.ProxyMode_PROXY_MODE_LITE
						}
						return &apipb.GetStatusResponse{
							Version: version.String(),
							Mode:    mode,
							Players: int32(players),
							Servers: int32(len(initialEnable.Servers())),
						}, nil
					},
					GetConfig: func(ctx context.Context, req *apipb.GetConfigRequest) (*apipb.GetConfigResponse, error) {
						cfgCopy := redactConfig(cfg)
						format := req.GetFormat()
						var (
							payload string
							err     error
							actual  apipb.ConfigFormat
						)
						switch format {
						case apipb.ConfigFormat_CONFIG_FORMAT_UNSPECIFIED, apipb.ConfigFormat_CONFIG_FORMAT_JSON:
							data, err := json.Marshal(cfgCopy)
							if err == nil {
								payload = string(data)
							}
							actual = apipb.ConfigFormat_CONFIG_FORMAT_JSON
						case apipb.ConfigFormat_CONFIG_FORMAT_YAML:
							data, err := yaml.Marshal(cfgCopy)
							if err == nil {
								payload = string(data)
							}
							actual = apipb.ConfigFormat_CONFIG_FORMAT_YAML
						default:
							return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported config format %v", format))
						}
						if err != nil {
							return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to encode config: %w", err))
						}
						return &apipb.GetConfigResponse{Format: actual, Payload: payload}, nil
					},
					ValidateConfig: func(ctx context.Context, req *apipb.ValidateConfigRequest) ([]string, error) {
						newCfg, err := decodeConfigOneof(req.GetYamlConfig(), req.GetJsonConfig())
						if err != nil {
							return nil, err
						}
						warns, errs := newCfg.Validate()
						if err := validationError("config validation failed", errs); err != nil {
							return nil, err
						}
						return errorsToStrings(warns), nil
					},
					ApplyConfig: func(ctx context.Context, req *apipb.ApplyConfigRequest) ([]string, error) {
						newCfg, err := decodeConfigOneof(req.GetYamlConfig(), req.GetJsonConfig())
						if err != nil {
							return nil, err
						}

						warns, err := applyConfigUpdate(ctx, newCfg, "applied config via api")
						if err != nil {
							return nil, err
						}

						// If persist is enabled, try to write the config to disk
						if req.GetPersist() {
							if err := persistConfig(&newCfg); err != nil {
								log.Error(err, "failed to persist config to disk (config applied in-memory)")
								// Don't fail the whole operation, just log the error
								warns = append(warns, fmt.Sprintf("failed to persist config to disk: %v", err))
							} else {
								log.Info("config persisted to disk")
							}
						}

						return warns, nil
					},
					ListLiteRoutes: func(ctx context.Context, _ *apipb.ListLiteRoutesRequest) (*apipb.ListLiteRoutesResponse, error) {
						mu.Lock()
						routes := make([]config2.Route, len(cfg.Config.Lite.Routes))
						copy(routes, cfg.Config.Lite.Routes)
						mu.Unlock()
						resp := &apipb.ListLiteRoutesResponse{}
						for _, r := range routes {
							resp.Routes = append(resp.Routes, toProtoRoute(r))
						}
						return resp, nil
					},
					GetLiteRoute: func(ctx context.Context, req *apipb.GetLiteRouteRequest) (*apipb.GetLiteRouteResponse, error) {
						host := strings.TrimSpace(req.GetHost())
						if host == "" {
							return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host is required"))
						}
						mu.Lock()
						defer mu.Unlock()
						idx := findRouteIdx(cfg, host)
						if idx < 0 {
							return nil, connect.NewError(connect.CodeNotFound, errors.New("route not found"))
						}
						return &apipb.GetLiteRouteResponse{Route: toProtoRoute(cfg.Config.Lite.Routes[idx])}, nil
					},
					UpdateLiteRouteStrategy: func(ctx context.Context, req *apipb.UpdateLiteRouteStrategyRequest) ([]string, error) {
						host := strings.TrimSpace(req.GetHost())
						if host == "" {
							return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host is required"))
						}
						strategy := strings.TrimSpace(req.GetStrategy())
						if strategy == "" {
							return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("strategy is required"))
						}
						mu.Lock()
						newCfg := *cfg
						mu.Unlock()
						idx := findRouteIdx(&newCfg, host)
						if idx < 0 {
							return nil, connect.NewError(connect.CodeNotFound, errors.New("route not found"))
						}
						old := string(newCfg.Config.Lite.Routes[idx].Strategy)
						newCfg.Config.Lite.Routes[idx].Strategy = config2.Strategy(strategy)
						return applyConfigUpdate(ctx, newCfg, "lite route strategy updated", "host", host, "old", old, "new", strategy)
					},
					AddLiteRouteBackend: func(ctx context.Context, req *apipb.AddLiteRouteBackendRequest) ([]string, error) {
						host := strings.TrimSpace(req.GetHost())
						backend := strings.TrimSpace(req.GetBackend())
						if host == "" || backend == "" {
							return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host and backend are required"))
						}
						mu.Lock()
						newCfg := *cfg
						mu.Unlock()
						idx := findRouteIdx(&newCfg, host)
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
						return applyConfigUpdate(ctx, newCfg, "lite route backend added", "host", host, "backend", backend, "alreadyExisted", exists)
					},
					RemoveLiteRouteBackend: func(ctx context.Context, req *apipb.RemoveLiteRouteBackendRequest) ([]string, error) {
						host := strings.TrimSpace(req.GetHost())
						backend := strings.TrimSpace(req.GetBackend())
						if host == "" || backend == "" {
							return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host and backend are required"))
						}
						mu.Lock()
						newCfg := *cfg
						mu.Unlock()
						idx := findRouteIdx(&newCfg, host)
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
						return applyConfigUpdate(ctx, newCfg, "lite route backend removed", "host", host, "backend", backend, "removed", removed)
					},
					UpdateLiteRouteOptions: func(ctx context.Context, req *apipb.UpdateLiteRouteOptionsRequest) ([]string, error) {
						if req.GetOptions() == nil {
							return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("options payload is required"))
						}
						host := strings.TrimSpace(req.GetHost())
						if host == "" {
							return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host is required"))
						}
						mu.Lock()
						newCfg := *cfg
						mu.Unlock()
						idx := findRouteIdx(&newCfg, host)
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
						return applyConfigUpdate(ctx, newCfg, "lite route options updated", "host", host)
					},
					UpdateLiteRouteFallback: func(ctx context.Context, req *apipb.UpdateLiteRouteFallbackRequest) ([]string, error) {
						host := strings.TrimSpace(req.GetHost())
						if host == "" {
							return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host is required"))
						}
						mu.Lock()
						newCfg := *cfg
						mu.Unlock()
						idx := findRouteIdx(&newCfg, host)
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
									motd, err := parseMOTD(req.GetFallback().GetMotdJson())
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
						return applyConfigUpdate(ctx, newCfg, "lite route fallback updated", "host", host)
					},
				}

				svc := api.NewService(initialEnable, handlers)
				srv := api.NewServer(c.Config.API.Config, svc)

				var runCtx context.Context
				runCtx, stop = context.WithCancel(ctx)
				go func() {
					if err := srv.Start(runCtx); err != nil {
						log.Error(err, "failed to start api service")
						return
					}
					log.Info("api service stopped")
				}()
			}
		}

		defer reload.Subscribe(eventMgr, trigger)()

		trigger(&reload.ConfigUpdateEvent[config.Config]{
			Config:     cfg,
			PrevConfig: cfg,
		})

		<-ctx.Done()
		return nil
	})
}
