package config

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"time"

	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/util/netutil"
)

// DefaultConfig is the default configuration for Lite mode.
var DefaultConfig = Config{
	Enabled: false,
	Routes:  []Route{},
}

type (
	// Config is the configuration for Lite mode.
	Config struct {
		Enabled bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
		Routes  []Route `yaml:"routes,omitempty" json:"routes,omitempty"`
	}
	Route struct {
		Host          configutil.SingleOrMulti[string] `json:"host,omitempty" yaml:"host,omitempty"`
		Backend       configutil.SingleOrMulti[string] `json:"backend,omitempty" yaml:"backend,omitempty"`
		CachePingTTL  configutil.Duration              `json:"cachePingTTL,omitempty" yaml:"cachePingTTL,omitempty"` // 0 = default, < 0 = disabled
		Fallback      *Status                          `json:"fallback,omitempty" yaml:"fallback,omitempty"`         // nil = disabled
		ProxyProtocol bool                             `json:"proxyProtocol,omitempty" yaml:"proxyProtocol,omitempty"`
		// Deprecated: use TCPShieldRealIP instead.
		RealIP            bool       `json:"realIP,omitempty" yaml:"realIP,omitempty"`
		TCPShieldRealIP   bool       `json:"tcpShieldRealIP,omitempty" yaml:"tcpShieldRealIP,omitempty"`
		ModifyVirtualHost bool       `json:"modifyVirtualHost,omitempty" yaml:"modifyVirtualHost,omitempty"`
		Strategy          Strategy   `json:"strategy,omitempty" yaml:"strategy,omitempty"`
		VoiceChat         *VoiceChat `json:"voiceChat,omitempty" yaml:"voiceChat,omitempty"` // nil = disabled
	}
	Status struct {
		MOTD    *configutil.Component `yaml:"motd,omitempty" json:"motd,omitempty"`
		Version ping.Version          `yaml:"version,omitempty" json:"version,omitempty"`
		Players *ping.Players         `json:"players,omitempty" yaml:"players,omitempty"`
		Favicon favicon.Favicon       `yaml:"favicon,omitempty" json:"favicon,omitempty"`
		ModInfo modinfo.ModInfo       `yaml:"modInfo,omitempty" json:"modInfo,omitempty"`
	}
)

// DefaultVoiceChatPort is the default UDP port of the Simple Voice Chat mod.
const DefaultVoiceChatPort = 24454

// VoiceChat enables relaying Simple Voice Chat UDP traffic for a route.
type VoiceChat struct {
	// Listen is the public UDP address Gate binds. Default ":24454".
	Listen string `json:"listen,omitempty" yaml:"listen,omitempty"`
	// BackendPort is the UDP voice port of the backends. Default 24454.
	BackendPort int `json:"backendPort,omitempty" yaml:"backendPort,omitempty"`
}

// ListenAddr returns the configured or default UDP listen address.
func (v *VoiceChat) ListenAddr() string {
	if v == nil || v.Listen == "" {
		return ":" + strconv.Itoa(DefaultVoiceChatPort)
	}
	return v.Listen
}

// BackendUDPPort returns the configured or default backend UDP port.
func (v *VoiceChat) BackendUDPPort() int {
	if v == nil || v.BackendPort == 0 {
		return DefaultVoiceChatPort
	}
	return v.BackendPort
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

func validListen(addr string) bool {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	p, err := strconv.Atoi(port)
	return err == nil && validPort(p)
}

// Response returns the configured status response.
func (s *Status) Response(proto.Protocol) (*ping.ServerPing, error) {
	return &ping.ServerPing{
		Version:     s.Version,
		Players:     s.Players,
		Description: s.MOTD.C(),
		Favicon:     s.Favicon,
		ModInfo:     &s.ModInfo,
	}, nil
}

// GetCachePingTTL returns the configured ping cache TTL or a default duration if not set.
func (r *Route) GetCachePingTTL() time.Duration {
	const defaultTTL = time.Second * 10
	if r.CachePingTTL == 0 {
		return defaultTTL
	}
	return time.Duration(r.CachePingTTL)
}

// CachePingEnabled returns true if the route has a ping cache enabled.
func (r *Route) CachePingEnabled() bool { return r.GetCachePingTTL() > 0 }

// GetTCPShieldRealIP returns the configured TCPShieldRealIP or deprecated RealIP value.
func (r *Route) GetTCPShieldRealIP() bool { return r.TCPShieldRealIP || r.RealIP }

// Strategy represents a load balancing strategy for lite mode routes.
type Strategy string

const (
	// StrategySequential selects backends in config order for each connection attempt.
	StrategySequential Strategy = "sequential"

	// StrategyRandom selects a random backend from available options.
	StrategyRandom Strategy = "random"

	// StrategyRoundRobin cycles through backends in order for each new connection.
	StrategyRoundRobin Strategy = "round-robin"

	// StrategyLeastConnections selects the backend with the fewest active connections.
	StrategyLeastConnections Strategy = "least-connections"

	// StrategyLowestLatency selects the backend with the lowest ping response time.
	StrategyLowestLatency Strategy = "lowest-latency"
)

var allowedStrategies = []Strategy{
	StrategySequential,
	StrategyRandom,
	StrategyRoundRobin,
	StrategyLeastConnections,
	StrategyLowestLatency,
}

func (c Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }
	w := func(m string, args ...any) { warns = append(warns, fmt.Errorf(m, args...)) }

	if len(c.Routes) == 0 {
		e("No routes configured")
		return
	}

	voiceListens := map[string]int{} // listen address -> backend UDP port
	for i, ep := range c.Routes {
		if len(ep.Host) == 0 {
			e("Route %d: no host configured", i)
		}
		if len(ep.Backend) == 0 {
			e("Route %d: no backend configured", i)
		}
		if !slices.Contains(allowedStrategies, ep.Strategy) && ep.Strategy != "" {
			e("Route %d: invalid strategy '%s', allowed: %v", i, ep.Strategy, allowedStrategies)
		}

		// Validate parameter usage in backend addresses
		for hostIdx, host := range ep.Host {
			wildcardCount := countWildcards(host)
			for backendIdx, addr := range ep.Backend {
				paramIndices := extractParameterIndices(addr)
				if len(paramIndices) > 0 {
					// Check if parameters exceed available wildcards
					maxParam := 0
					for _, idx := range paramIndices {
						if idx > maxParam {
							maxParam = idx
						}
					}
					if maxParam > wildcardCount {
						w("Route %d: host %d '%s' has %d wildcard(s) but backend %d '%s' uses parameter $%d (parameters will not be substituted)",
							i, hostIdx, host, wildcardCount, backendIdx, addr, maxParam)
					}
					// Warn if no wildcards but parameters are used
					if wildcardCount == 0 {
						w("Route %d: host %d '%s' has no wildcards but backend %d '%s' uses parameters (parameters will not be substituted)",
							i, hostIdx, host, backendIdx, addr)
					}
				}

				// Validate address parsing (after parameter substitution would happen)
				// We can't fully validate addresses with parameters, but we can check the structure
				_, err := netutil.Parse(addr, "tcp")
				if err != nil {
					// If it contains parameters, it might be valid after substitution
					if !containsParameters(addr) {
						e("Route %d: backend %d: failed to parse address: %w", i, backendIdx, err)
					}
				}
			}
		}

		if v := ep.VoiceChat; v != nil {
			if v.Listen != "" && !validListen(v.Listen) {
				e("Route %d: invalid voiceChat.listen '%s', expected host:port with port 1-65535", i, v.Listen)
			} else if v.BackendPort != 0 && !validPort(v.BackendPort) {
				e("Route %d: invalid voiceChat.backendPort %d, expected 1-65535", i, v.BackendPort)
			} else {
				listen, bp := v.ListenAddr(), v.BackendUDPPort()
				if prev, ok := voiceListens[listen]; ok && prev != bp {
					e("Route %d: routes that share listen '%s' must use the same voiceChat.backendPort", i, listen)
				}
				voiceListens[listen] = bp
			}
		}
	}

	return
}

// countWildcards counts the number of wildcard characters (* and ?) in a pattern.
func countWildcards(pattern string) int {
	count := 0
	escapeNext := false
	for _, r := range pattern {
		if escapeNext {
			escapeNext = false
			continue
		}
		if r == '\\' {
			escapeNext = true
			continue
		}
		if r == '*' || r == '?' {
			count++
		}
	}
	return count
}

// extractParameterIndices extracts all parameter indices ($1, $2, etc.) from a string.
// Returns a slice of unique parameter indices found.
func extractParameterIndices(s string) []int {
	// Match $ followed by one or more digits
	re := regexp.MustCompile(`\$(\d+)`)
	matches := re.FindAllStringSubmatch(s, -1)

	indices := make(map[int]bool)
	for _, match := range matches {
		if len(match) > 1 {
			if idx, err := strconv.Atoi(match[1]); err == nil {
				indices[idx] = true
			}
		}
	}

	result := make([]int, 0, len(indices))
	for idx := range indices {
		result = append(result, idx)
	}
	return result
}

// containsParameters returns true if the string contains parameter placeholders like $1, $2, etc.
func containsParameters(s string) bool {
	matched, _ := regexp.MatchString(`\$\d+`, s)
	return matched
}

// Equal returns true if the Routes are equal.
func (r *Route) Equal(other *Route) bool {
	j, err := json.Marshal(r)
	if err != nil {
		return false
	}
	o, err := json.Marshal(other)
	if err != nil {
		return false
	}
	return string(j) == string(o)
}
