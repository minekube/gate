package proxy

import (
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/auth"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/event"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/packet/plugin"
	"go.minekube.com/gate/pkg/proxy/message"
	"go.minekube.com/gate/pkg/util/sets"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"strings"
	"sync"
)

type Proxy struct {
	config           *config.Config
	event            *event.Manager
	connect          *Connect
	channelRegistrar *ChannelRegistrar
	authenticator    *auth.Authenticator

	shuttingDown atomic.Bool

	smu     sync.RWMutex                // Protects following fields
	servers map[string]RegisteredServer // registered backend servers: by lower case names
}

// NewProxy returns a new initialized proxy server.
func NewProxy(config *config.Config) (s *Proxy) {
	defer func() {
		s.connect = NewConnect(s)
	}()
	return &Proxy{
		config:           config,
		event:            event.NewManager(),
		channelRegistrar: NewChannelRegistrar(),
		servers:          map[string]RegisteredServer{},
		authenticator:    auth.NewAuthenticator(),
	}
}

func (s *Proxy) Event() *event.Manager {
	return s.event
}

func (s *Proxy) Config() *config.Config {
	return s.config
}

func (s *Proxy) Connect() *Connect {
	return s.connect
}

func (s *Proxy) Run() error {
	for name, addr := range s.Config().Servers {
		s.Register(NewServerInfo(name, tcpAddr(addr)))
	}
	return s.connect.listen(s.Config().Bind)
}

func (s *Proxy) Shutdown(reason component.Component) {
	if !s.shuttingDown.CAS(false, true) {
		return // Already shutdown
	}
	zap.L().Info("Shutting down the proxy...")
	// Shutdown the connection manager, this should be
	// done first to refuse new connections.
	s.connect.closeListener()
	s.connect.DisconnectAll(reason)
	s.event.Wait()
}

// Server gets a backend server registered with the proxy by name.
// Returns nil if not found.
func (s *Proxy) Server(name string) RegisteredServer {
	name = strings.ToLower(name)
	s.smu.RLock()
	defer s.smu.RUnlock()
	rs, _ := s.servers[name]
	return rs // may be nil
}

// Servers gets all registered servers.
func (s *Proxy) Servers() []RegisteredServer {
	s.smu.RLock()
	defer s.smu.RUnlock()
	l := make([]RegisteredServer, 0, len(s.servers))
	for _, rs := range s.servers {
		l = append(l, rs)
	}
	return l
}

// Register registers a server with the proxy.
//
// Returns the new registered server and true on success.
// On failure returns false and the already registered server with the same name.
func (s *Proxy) Register(info ServerInfo) (RegisteredServer, bool) {
	name := strings.ToLower(info.Name())

	s.smu.Lock()
	defer s.smu.Unlock()
	if exists, ok := s.servers[name]; ok {
		return exists, false
	}
	rs := newRegisteredServer(info)
	s.servers[name] = rs
	return rs, true
}

// Unregister unregisters the server exactly matching the
// given ServerInfo and returns true if found.
func (s *Proxy) Unregister(info ServerInfo) bool {
	name := strings.ToLower(info.Name())
	s.smu.Lock()
	defer s.smu.Unlock()
	rs, ok := s.servers[name]
	if !ok || !rs.ServerInfo().Equals(info) {
		return false
	}
	delete(s.servers, name)
	return true
}

func (s *Proxy) ChannelRegistrar() *ChannelRegistrar {
	return s.channelRegistrar
}

//
//
//
//
//
//

type ChannelRegistrar struct {
	mu          sync.RWMutex // Protects following fields
	identifiers map[string]message.ChannelIdentifier
}

func NewChannelRegistrar() *ChannelRegistrar {
	return &ChannelRegistrar{identifiers: map[string]message.ChannelIdentifier{}}
}

// ChannelsForProtocol returns all the channel names
// to register depending on the Minecraft protocol version.
func (r *ChannelRegistrar) ChannelsForProtocol(protocol proto.Protocol) sets.String {
	if protocol.GreaterEqual(proto.Minecraft_1_13) {
		return r.ModernChannelIds()
	}
	return r.LegacyChannelIds()
}

// ModernChannelIds returns all channel IDs (as strings)
// for use with Minecraft 1.13 and above.
func (r *ChannelRegistrar) ModernChannelIds() sets.String {
	r.mu.RLock()
	ids := r.identifiers
	r.mu.RUnlock()
	ss := sets.String{}
	for _, i := range ids {
		if _, ok := i.(*message.MinecraftChannelIdentifier); ok {
			ss.Insert(i.Id())
		} else {
			ss.Insert(plugin.TransformLegacyToModernChannel(i.Id()))
		}
	}
	return ss
}

// LegacyChannelIds returns all legacy channel IDs.
func (r *ChannelRegistrar) LegacyChannelIds() sets.String {
	r.mu.RLock()
	ids := r.identifiers
	r.mu.RUnlock()
	ss := sets.String{}
	for _, i := range ids {
		ss.Insert(i.Id())
	}
	return ss
}

func (r *ChannelRegistrar) FromId(channel string) (message.ChannelIdentifier, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.identifiers[channel]
	return id, ok
}
