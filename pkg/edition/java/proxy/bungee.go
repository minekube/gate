package proxy

import (
	"context"
	"errors"
	"net"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/bungeecord"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/gate/proto"
)

func newBungeeCordMessageResponder(
	bungeePluginChannelEnabled bool,
	player *connectedPlayer,
	proxy *Proxy,
) bungeecord.MessageResponder {
	if !bungeePluginChannelEnabled {
		return bungeecord.NopMessageResponder
	}
	adapter := &bungeeMessageResponderAdapter{
		player: player,
		Proxy:  proxy,
	}
	return bungeecord.NewMessageResponder(player, adapter)
}

type (
	bungeeServer struct {
		proxy *Proxy
		s     RegisteredServer
		smc   netmc.MinecraftConn
	}
	bungeeMessageResponderAdapter struct {
		player *connectedPlayer // the associated player
		*Proxy
	}
)

var (
	_ bungeecord.ServerConnection = (*bungeeServer)(nil)
	_ bungeecord.Server           = (*bungeeServer)(nil)
	_ bungeecord.Providers        = (*bungeeMessageResponderAdapter)(nil)
)

func (s *bungeeServer) PlayerCount() int {
	if s == nil {
		return 0
	}
	return s.s.Players().Len()
}
func (s *bungeeServer) BroadcastPluginMessage(identifier message.ChannelIdentifier, data []byte) {
	if s == nil {
		return
	}
	sinks := PlayersToSlice[message.ChannelMessageSink](s.s.Players())
	BroadcastPluginMessage(sinks, identifier, data)
}
func (s *bungeeServer) Connect(player bungeecord.Player) {
	if s == nil {
		return
	}
	p := s.proxy.Player(player.ID())
	if p == nil {
		return
	}
	timeout := time.Duration(s.proxy.config().ConnectionTimeout) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	_ = p.CreateConnectionRequest(s.s).ConnectWithIndication(ctx)
}
func (s *bungeeServer) Players() []bungeecord.Player {
	if s == nil {
		return nil
	}
	return PlayersToSlice[bungeecord.Player](s.s.Players())
}

func (s *bungeeServer) BroadcastMessage(comp component.Component) {
	if s == nil {
		return
	}
	sinks := PlayersToSlice[MessageSink](s.s.Players())
	BroadcastMessage(sinks, comp)
}
func (s *bungeeServer) Addr() net.Addr {
	if s == nil {
		return nil
	}
	return s.s.ServerInfo().Addr()
}
func (s *bungeeServer) Name() string {
	if s == nil {
		return ""
	}
	return s.s.ServerInfo().Name()
}
func (s *bungeeServer) Protocol() proto.Protocol {
	if s == nil {
		return version.Unknown.Protocol
	}
	return s.smc.Protocol()
}
func (s *bungeeServer) WritePacket(packet proto.Packet) error {
	if s == nil {
		return errors.New("server is nil")
	}
	return s.smc.WritePacket(packet)
}

func (b *bungeeMessageResponderAdapter) PlayerByName(username string) bungeecord.Player {
	return b.Proxy.PlayerByName(username)
}
func (b *bungeeMessageResponderAdapter) Players() []bungeecord.Player {
	return convertSlice[bungeecord.Player](b.Proxy.Players())
}
func (b *bungeeMessageResponderAdapter) BroadcastMessage(comp component.Component) {
	sinks := convertSlice[MessageSink](b.Proxy.Players())
	BroadcastMessage(sinks, comp)
}
func (b *bungeeMessageResponderAdapter) Server(name string) bungeecord.Server {
	s := b.Proxy.Server(name)
	if s == nil {
		return nil
	}
	return &bungeeServer{
		proxy: b.Proxy,
		s:     s,
	}
}
func (b *bungeeMessageResponderAdapter) Servers() []bungeecord.Server {
	servers := b.Proxy.Servers()
	bungeeServers := make([]bungeecord.Server, len(servers))
	for i, s := range servers {
		bungeeServers[i] = &bungeeServer{
			proxy: b.Proxy,
			s:     s,
		}
	}
	return bungeeServers
}
func (b *bungeeMessageResponderAdapter) ConnectedServer() bungeecord.ServerConnection {
	server := b.player.connectedServer()
	if server == nil {
		return nil
	}
	smc, ok := server.ensureConnected()
	if !ok {
		return nil
	}
	return &bungeeServer{
		proxy: b.Proxy,
		s:     server.Server(),
		smc:   smc,
	}
}

func convertSlice[T any](a []Player) []T {
	b := make([]T, len(a))
	for i, v := range a {
		t, ok := v.(T)
		if ok {
			b[i] = t
		}
	}
	return b
}
