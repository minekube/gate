package proxy

import (
	"bytes"
	"context"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/packet/plugin"
	"go.minekube.com/gate/pkg/proto/util"
	"go.minekube.com/gate/pkg/proxy/message"
	codecutil "go.minekube.com/gate/pkg/util"
	"go.minekube.com/gate/pkg/util/uuid"
	"io"
	"net"
	"strconv"
	"strings"
)

type bungeeCordMessageRecorder struct {
	*connectedPlayer
}

var (
	bungeeCordModernChannel = &message.MinecraftChannelIdentifier{Key: key.New("bungeecord", "main")}
	bungeeCordLegacyChannel = message.LegacyChannelIdentifier("BungeeCord")
)

func (r *bungeeCordMessageRecorder) bungeeCordChannel(protocol proto.Protocol) string {
	if protocol.GreaterEqual(proto.Minecraft_1_13) {
		return bungeeCordModernChannel.Id()
	}
	return bungeeCordLegacyChannel.Id()
}

func (r *bungeeCordMessageRecorder) config() *config.Config {
	return r.proxy.config
}

func (r *bungeeCordMessageRecorder) process(message *plugin.Message) bool {
	if !r.config().BungeePluginChannelEnabled {
		return false
	}

	if !strings.EqualFold(bungeeCordModernChannel.Id(), message.Channel) &&
		!strings.EqualFold(bungeeCordLegacyChannel.Id(), message.Channel) {
		return false
	}

	in := bytes.NewReader(message.Data)
	subChannel, err := util.ReadUTF(in) // read first sequence
	if err != nil {
		return false
	}
	switch subChannel {
	case "ForwardToPlayer":
		r.processForwardToPlayer(in)
	case "Forward":
		r.processForwardToServer(in)
	case "Connect":
		r.processConnect(in)
	case "ConnectOther":
		r.processConnectOther(in)
	case "IP":
		r.processIP()
	case "UUID":
		r.processUUID()
	case "UUIDOther":
		r.processUUIDOther(in)
	case "PlayerCount":
		r.processPlayerCount(in)
	case "PlayerList":
		r.processPlayerList(in)
	case "GetServers":
		r.processGetServers()
	case "GetServer":
		r.processGetServer()
	case "Message":
		r.processMessage(in)
	case "MessageRaw":
		r.processMessageRaw(in)
	case "ServerIP":
		r.processServerIP(in)
	case "KickPlayer":
		r.processKick(in)
	default:
		// Unknown sub-channel, do nothing
	}
	return true
}

func (r *bungeeCordMessageRecorder) prepareForwardMessage(in io.Reader) (forward []byte) {
	channel, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	messageLen, err := util.ReadInt16(in)
	if err != nil {
		return
	}
	msg := make([]byte, messageLen)
	_, err = io.ReadFull(in, msg)
	if err != nil {
		return
	}

	forwarded := new(bytes.Buffer)
	forwarded.WriteString(channel)
	_ = util.WriteInt16(forwarded, messageLen)
	forwarded.Write(msg)
	return forwarded.Bytes()
}

func (r *bungeeCordMessageRecorder) sendServerResponse(in []byte) {
	if len(in) == 0 {
		return
	}
	serverConn, ok := r.connectedServer().ensureConnected()
	if !ok {
		return
	}
	ch := r.bungeeCordChannel(serverConn.protocol)
	_ = serverConn.WritePacket(&plugin.Message{Channel: ch, Data: in})
}

func (r *bungeeCordMessageRecorder) processForwardToPlayer(in io.Reader) {
	r.readPlayer(in, func(player *connectedPlayer) {
		serverConn := player.CurrentServer()
		if serverConn == nil {
			return
		}
		r.sendServerResponse(r.prepareForwardMessage(in))
	})
}

func (r *bungeeCordMessageRecorder) processForwardToServer(in io.Reader) {
	target, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	forward := r.prepareForwardMessage(in)
	if strings.EqualFold(target, "ALL") {
		r.proxy.mu.RLock()
		servers := r.proxy.servers
		r.proxy.mu.RUnlock()
		for _, server := range servers {
			go server.sendPluginMessage(bungeeCordLegacyChannel, forward)
		}
	} else {
		r.proxy.server(target).sendPluginMessage(bungeeCordLegacyChannel, forward)
	}
}

func (r *bungeeCordMessageRecorder) processConnect(in io.Reader) {
	r.readServer(in, func(s *registeredServer) {
		ctx, cancel := withConnectionTimeout(context.Background(), r.config())
		defer cancel()
		r.CreateConnectionRequest(s).ConnectWithIndication(ctx)
	})
}

func (r *bungeeCordMessageRecorder) processConnectOther(in io.Reader) {
	r.readPlayer(in, func(player *connectedPlayer) {
		r.readServer(in, func(server *registeredServer) {
			ctx, cancel := withConnectionTimeout(context.Background(), r.config())
			defer cancel()
			player.CreateConnectionRequest(server).ConnectWithIndication(ctx)
		})
	})
}

func (r *bungeeCordMessageRecorder) processIP() {
	ip, portS, err := net.SplitHostPort(r.RemoteAddr().String())
	if err != nil {
		return
	}
	port, err := strconv.Atoi(portS)
	if err != nil {
		return
	}
	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "IP")
	_ = util.WriteUTF(b, ip)
	_ = util.WriteInt32(b, int32(port))
	r.sendServerResponse(b.Bytes())
}

func (r *bungeeCordMessageRecorder) processPlayerCount(in io.Reader) {
	target, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	var (
		count int
		name  = "ALL"
	)
	if strings.EqualFold(target, name) {
		count = r.proxy.PlayerCount()
	} else {
		s := r.proxy.Server(target)
		if s == nil {
			return
		}
		name = s.ServerInfo().Name()
		count = s.Players().Len()
	}

	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "PlayerCount")
	_ = util.WriteUTF(b, name)
	_ = util.WriteInt32(b, int32(count))

	r.sendServerResponse(b.Bytes())
}

func (r *bungeeCordMessageRecorder) processPlayerList(in io.Reader) {
	target, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	var (
		name    = "ALL"
		players map[uuid.UUID]*connectedPlayer
	)
	if strings.EqualFold(target, name) {
		r.proxy.mu.RLock()
		players = r.proxy.ids
		r.proxy.mu.RUnlock()
	} else {
		s := r.proxy.server(target)
		if s == nil {
			return
		}
		name = s.ServerInfo().Name()
		s.players.mu.RLock()
		players = s.players.list
		s.players.mu.RUnlock()
	}

	list := joiner{split: ", "}
	for _, p := range players {
		list.Add(p.Username())
	}

	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "PlayerList")
	_ = util.WriteUTF(b, name)
	_ = util.WriteUTF(b, list.String())

	r.sendServerResponse(b.Bytes())
}

func (r *bungeeCordMessageRecorder) processGetServers() {
	r.proxy.mu.RLock()
	servers := r.proxy.servers
	r.proxy.mu.RUnlock()

	list := joiner{split: ", "}
	for _, s := range servers {
		list.Add(s.ServerInfo().Name())
	}
	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "GetServers")
	_ = util.WriteUTF(b, list.String())
}

func (r *bungeeCordMessageRecorder) processMessage0(in io.Reader, decoder codec.Unmarshaler) {
	target, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	msg, err := util.ReadUTF(in)
	if err != nil {
		return
	}

	comp, err := decoder.Unmarshal([]byte(msg))
	if err != nil {
		return
	}
	if strings.EqualFold(target, "ALL") {
		r.proxy.sendMessage(comp)
	} else {
		r.proxy.Server(target).Players().Range(func(p Player) bool {
			go func(p Player) { _ = p.SendMessage(comp) }(p)
			return true
		})
	}
}
func (r *bungeeCordMessageRecorder) processMessage(in io.Reader) {
	r.processMessage0(in, &legacy.Legacy{})
}
func (r *bungeeCordMessageRecorder) processMessageRaw(in io.Reader) {
	r.processMessage0(in, codecutil.DefaultJsonCodec())
}

func (r *bungeeCordMessageRecorder) processGetServer() {
	s := r.connectedServer()
	if s == nil {
		return
	}
	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "GetServer")
	_ = util.WriteUTF(b, s.Server().ServerInfo().Name())
	r.sendServerResponse(b.Bytes())
}

func (r *bungeeCordMessageRecorder) processUUID() {
	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "UUID")
	_ = util.WriteUTF(b, r.Id().Undashed())
	r.sendServerResponse(b.Bytes())
}
func (r *bungeeCordMessageRecorder) processUUIDOther(in io.Reader) {
	r.readPlayer(in, func(player *connectedPlayer) {
		b := new(bytes.Buffer)
		_ = util.WriteUTF(b, "UUIDOther")
		_ = util.WriteUTF(b, player.Username())
		_ = util.WriteUTF(b, player.Id().Undashed())
		r.sendServerResponse(b.Bytes())
	})
}

func (r *bungeeCordMessageRecorder) processServerIP(in io.Reader) {
	r.readServer(in, func(s *registeredServer) {
		host, portS, err := net.SplitHostPort(s.ServerInfo().Addr().String())
		if err != nil {
			return
		}
		port, err := strconv.Atoi(portS)
		if err != nil {
			return
		}

		b := new(bytes.Buffer)
		_ = util.WriteUTF(b, "ServerIP")
		_ = util.WriteUTF(b, s.ServerInfo().Name())
		_ = util.WriteUTF(b, host)
		_ = util.WriteInt16(b, int16(port))
		r.sendServerResponse(b.Bytes())
	})
}

func (r *bungeeCordMessageRecorder) processKick(in io.Reader) {
	r.readPlayer(in, func(p *connectedPlayer) {
		msg, err := util.ReadUTF(in)
		if err != nil {
			return
		}
		kickReason, err := (&legacy.Legacy{}).Unmarshal([]byte(msg))
		if err != nil {
			kickReason = &component.Text{} // fallback to blank reason
		}
		p.Disconnect(kickReason)
	})
}

//
//
//

func (r *bungeeCordMessageRecorder) readServer(in io.Reader, fn func(s *registeredServer)) {
	name, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	server := r.proxy.server(name)
	if server != nil {
		fn(server)
	}
}

func (r *bungeeCordMessageRecorder) readPlayer(in io.Reader, fn func(p *connectedPlayer)) {
	name, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	player := r.proxy.playerByName(name)
	if player != nil {
		fn(player)
	}
}

// joiner joins strings with a spliterator
type joiner struct {
	split string
	b     strings.Builder
}

func (j *joiner) Add(s string) {
	if j.b.Len() != 0 {
		j.b.WriteString(j.split)
	}
	j.b.WriteString(s)
}

func (j *joiner) String() string {
	return j.b.String()
}
