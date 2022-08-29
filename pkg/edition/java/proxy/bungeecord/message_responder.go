package bungeecord

import (
	"bytes"
	"context"
	"io"
	"strings"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proxy"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/netutil"
)

// MessageResponder is a message responder for BungeeCord plugin channels.
type MessageResponder interface {
	// Process processes the given plugin message.
	// Returns true if the message is a BungeeCord plugin message and was processed.
	Process(message *plugin.Message) bool
}

// NopMessageResponder is a MessageResponder that does not process messages.
var NopMessageResponder MessageResponder = &nopMessageResponder{}

// Dependencies required by NewMessageResponder.
type (
	// PlayerProvider provides a player by its name.
	PlayerProvider interface {
		PlayerByName(username string) proxy.Player
		PlayerCount() int // Total number of players online
		Players() []proxy.Player
	}
	// ServerProvider provides servers.
	ServerProvider interface {
		Server(name string) proxy.RegisteredServer
		Servers() []proxy.RegisteredServer
	}
	// ServerConnectionProvider provides the currently connected server connection for a player.
	ServerConnectionProvider interface {
		ConnectedServer() ServerConnection
	}
	// ServerConnection represents a server connection for a player.
	ServerConnection interface {
		Name() string // Name of the server.
		Protocol() proto.Protocol
		proto.PacketWriter
	}
)

// NewMessageResponder returns a new MessageResponder.
func NewMessageResponder(
	player proxy.Player,
	connectionTimeout time.Duration,
	playerProvider PlayerProvider,
	serverProvider ServerProvider,
	serverConnectionProvider ServerConnectionProvider,
) MessageResponder {
	return &bungeeCordMessageResponder{
		Player:                   player,
		ConnectionTimeout:        connectionTimeout,
		PlayerProvider:           playerProvider,
		ServerProvider:           serverProvider,
		ServerConnectionProvider: serverConnectionProvider,
	}
}

type bungeeCordMessageResponder struct {
	Player                   proxy.Player // The player of this responder.
	ConnectionTimeout        time.Duration
	PlayerProvider           PlayerProvider
	ServerProvider           ServerProvider
	ServerConnectionProvider ServerConnectionProvider
}

var (
	bungeeCordModernChannel = (&message.MinecraftChannelIdentifier{Key: key.New("bungeecord", "main")}).ID()
	bungeeCordLegacyChannel = message.LegacyChannelIdentifier("BungeeCord")
)

// Channel returns the BungeeCord plugin channel identifier for the given protocol.
func Channel(protocol proto.Protocol) string {
	if protocol.GreaterEqual(version.Minecraft_1_13) {
		return bungeeCordModernChannel
	}
	return bungeeCordLegacyChannel.ID()
}

func (r *bungeeCordMessageResponder) Process(message *plugin.Message) bool {
	if !strings.EqualFold(bungeeCordModernChannel, message.Channel) &&
		!strings.EqualFold(bungeeCordLegacyChannel.ID(), message.Channel) {
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
	case "IPOther":
		r.processIPOther(in)
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

func (r *bungeeCordMessageResponder) prepareForwardMessage(in io.Reader) (forward []byte) {
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

func (r *bungeeCordMessageResponder) sendServerResponse(in []byte) {
	if len(in) == 0 {
		return
	}
	serverConn := r.ServerConnectionProvider.ConnectedServer()
	if serverConn == nil {
		return
	}
	ch := Channel(serverConn.Protocol())
	_ = serverConn.WritePacket(&plugin.Message{Channel: ch, Data: in})
}

func (r *bungeeCordMessageResponder) processForwardToPlayer(in io.Reader) {
	r.readPlayer(in, func(player proxy.Player) {
		r.sendServerResponse(r.prepareForwardMessage(in))
	})
}

func (r *bungeeCordMessageResponder) processForwardToServer(in io.Reader) {
	target, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	forward := r.prepareForwardMessage(in)
	if strings.EqualFold(target, "ALL") {
		var currentUserServer string
		if s := r.ServerConnectionProvider.ConnectedServer(); s != nil {
			currentUserServer = s.Name()
		}
		// Broadcast message to players on all servers except the current one
		for _, server := range r.ServerProvider.Servers() {
			if server.ServerInfo().Name() == currentUserServer {
				continue // skip current server
			}
			sinks := proxy.PlayersToSlice[message.ChannelMessageSink](server.Players())
			proxy.BroadcastPluginMessage(sinks, bungeeCordLegacyChannel, forward)
		}
	} else {
		if server := r.ServerProvider.Server(target); server != nil {
			sinks := proxy.PlayersToSlice[message.ChannelMessageSink](server.Players())
			proxy.BroadcastPluginMessage(sinks, bungeeCordLegacyChannel, forward)
		}
	}
}

func (r *bungeeCordMessageResponder) processConnect(in io.Reader) {
	r.readServer(in, func(server proxy.RegisteredServer) {
		r.connect(r.Player, server)
	})
}
func (r *bungeeCordMessageResponder) processConnectOther(in io.Reader) {
	r.readPlayer(in, func(player proxy.Player) {
		r.readServer(in, func(server proxy.RegisteredServer) {
			r.connect(player, server)
		})
	})
}
func (r *bungeeCordMessageResponder) connect(cr proxy.Player, server proxy.RegisteredServer) {
	ctx, cancel := context.WithTimeout(context.TODO(), r.ConnectionTimeout)
	defer cancel()
	_ = cr.CreateConnectionRequest(server).ConnectWithIndication(ctx)
}

func (r *bungeeCordMessageResponder) processIP() {
	host, port := netutil.HostPort(r.Player.RemoteAddr())
	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "IP")
	_ = util.WriteUTF(b, host)
	_ = util.WriteInt32(b, int32(port))
	r.sendServerResponse(b.Bytes())
}

func (r *bungeeCordMessageResponder) processIPOther(in io.Reader) {
	r.readPlayer(in, func(player proxy.Player) {
		host, port := netutil.HostPort(player.RemoteAddr())
		b := new(bytes.Buffer)
		_ = util.WriteUTF(b, "IPOther")
		_ = util.WriteUTF(b, player.Username())
		_ = util.WriteUTF(b, host)
		_ = util.WriteInt32(b, int32(port))
		r.sendServerResponse(b.Bytes())
	})
}

func (r *bungeeCordMessageResponder) processPlayerCount(in io.Reader) {
	target, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	var (
		count int
		name  = "ALL"
	)
	if strings.EqualFold(target, name) {
		count = r.PlayerProvider.PlayerCount()
	} else {
		s := r.ServerProvider.Server(target)
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

func (r *bungeeCordMessageResponder) processPlayerList(in io.Reader) {
	target, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	var (
		name    = "ALL"
		players []proxy.Player
	)
	if target == name {
		players = r.PlayerProvider.Players()
	} else {
		server := r.ServerProvider.Server(target)
		if server == nil {
			return
		}
		name = server.ServerInfo().Name()
		players = proxy.PlayersToSlice[proxy.Player](server.Players())
	}

	list := joiner{split: ", "}
	for _, player := range players {
		list.write(player.Username())
	}

	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "PlayerList")
	_ = util.WriteUTF(b, name)
	_ = util.WriteUTF(b, list.String())

	r.sendServerResponse(b.Bytes())
}

func (r *bungeeCordMessageResponder) processGetServers() {
	list := joiner{split: ", "}
	for _, server := range r.ServerProvider.Servers() {
		list.write(server.ServerInfo().Name())
	}
	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "GetServers")
	_ = util.WriteUTF(b, list.String())
}

func (r *bungeeCordMessageResponder) processMessage0(in io.Reader, decoder codec.Unmarshaler) {
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
		sinks := convertSlice[proxy.Player, proxy.MessageSink](r.PlayerProvider.Players())
		proxy.BroadcastMessage(sinks, comp)
	} else {
		players := proxy.PlayersToSlice[proxy.Player](r.ServerProvider.Server(target).Players())
		sinks := convertSlice[proxy.Player, proxy.MessageSink](players)
		proxy.BroadcastMessage(sinks, comp)
	}
}
func (r *bungeeCordMessageResponder) processMessage(in io.Reader) {
	r.processMessage0(in, &legacy.Legacy{})
}
func (r *bungeeCordMessageResponder) processMessageRaw(in io.Reader) {
	r.processMessage0(in, util.DefaultJsonCodec())
}

func (r *bungeeCordMessageResponder) processGetServer() {
	s := r.ServerConnectionProvider.ConnectedServer()
	if s == nil {
		return
	}
	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "GetServer")
	_ = util.WriteUTF(b, s.Name())
	r.sendServerResponse(b.Bytes())
}

func (r *bungeeCordMessageResponder) processUUID() {
	b := new(bytes.Buffer)
	_ = util.WriteUTF(b, "UUID")
	_ = util.WriteUTF(b, r.Player.ID().Undashed())
	r.sendServerResponse(b.Bytes())
}
func (r *bungeeCordMessageResponder) processUUIDOther(in io.Reader) {
	r.readPlayer(in, func(player proxy.Player) {
		b := new(bytes.Buffer)
		_ = util.WriteUTF(b, "UUIDOther")
		_ = util.WriteUTF(b, player.Username())
		_ = util.WriteUTF(b, player.ID().Undashed())
		r.sendServerResponse(b.Bytes())
	})
}

func (r *bungeeCordMessageResponder) processServerIP(in io.Reader) {
	r.readServer(in, func(server proxy.RegisteredServer) {
		host, port := netutil.HostPort(server.ServerInfo().Addr())
		b := new(bytes.Buffer)
		_ = util.WriteUTF(b, "ServerIP")
		_ = util.WriteUTF(b, server.ServerInfo().Name())
		_ = util.WriteUTF(b, host)
		_ = util.WriteInt16(b, int16(port))
		r.sendServerResponse(b.Bytes())
	})
}

func (r *bungeeCordMessageResponder) processKick(in io.Reader) {
	r.readPlayer(in, func(player proxy.Player) {
		msg, err := util.ReadUTF(in)
		if err != nil {
			return
		}
		kickReason, err := (&legacy.Legacy{}).Unmarshal([]byte(msg))
		if err != nil {
			kickReason = &component.Text{} // fallback to blank reason
		}
		player.Disconnect(kickReason)
	})
}

//
//
//

type (
	playerFn func(p proxy.Player)
	serverFn func(s proxy.RegisteredServer)
)

func (r *bungeeCordMessageResponder) readServer(in io.Reader, fn serverFn) {
	name, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	server := r.ServerProvider.Server(name)
	if server != nil {
		fn(server)
	}
}
func (r *bungeeCordMessageResponder) readPlayer(in io.Reader, fn playerFn) {
	name, err := util.ReadUTF(in)
	if err != nil {
		return
	}
	player := r.PlayerProvider.PlayerByName(name)
	if player != nil {
		fn(player)
	}
}

// joiner joins strings with a spliterator
type joiner struct {
	split string
	b     strings.Builder
}

func (j *joiner) write(s string) {
	if j.b.Len() != 0 {
		j.b.WriteString(j.split)
	}
	j.b.WriteString(s)
}

func (j *joiner) String() string {
	return j.b.String()
}

type nopMessageResponder struct{}

func (n *nopMessageResponder) Process(*plugin.Message) bool { return false }

var _ MessageResponder = (*nopMessageResponder)(nil)

func convertSlice[T any, R T](a []T) []R {
	b := make([]R, len(a))
	for i, v := range a {
		b[i] = v.(R)
	}
	return b
}
