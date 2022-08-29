package proxy

import (
	"time"

	"go.minekube.com/gate/pkg/edition/java/proxy/bungeecord"
	"go.minekube.com/gate/pkg/gate/proto"
)

func bungeeCordMessageResponder(
	bungeePluginChannelEnabled bool,
	player *connectedPlayer,
	proxy *Proxy,
) bungeecord.MessageResponder {
	if !bungeePluginChannelEnabled {
		return bungeecord.NopMessageResponder
	}
	return bungeecord.NewMessageResponder(
		player,
		time.Duration(proxy.config.ConnectionTimeout),
		proxy, proxy, &serverConnProvider{player},
	)
}

type (
	serverConnProvider struct{ *connectedPlayer }
	serverConn         struct {
		s   RegisteredServer
		smc *minecraftConn
	}
)

func (p *serverConnProvider) ConnectedServer() bungeecord.ServerConnection {
	server := p.connectedServer()
	if server == nil {
		return nil
	}
	smc, ok := server.ensureConnected()
	if !ok {
		return nil
	}
	return &serverConn{
		s:   server.Server(),
		smc: smc,
	}
}
func (s *serverConn) Name() string                          { return s.s.ServerInfo().Name() }
func (s *serverConn) Protocol() proto.Protocol              { return s.smc.Protocol() }
func (s *serverConn) WritePacket(packet proto.Packet) error { return s.smc.WritePacket(packet) }
