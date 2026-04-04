package proxy

import (
	"sync"

	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
)

// ForgeLoginWrapperChannel is the Forge login wrapper channel used for FML2/FML3
// mod negotiation during the LOGIN phase (Minecraft 1.13-1.20.1).
const ForgeLoginWrapperChannel = "fml:loginwrapper"

// modernForgeLoginRelay relays LoginPluginMessages between a backend Forge server
// and a client during the LOGIN phase. This enables Forge 1.13-1.20.1 mod negotiation
// through the proxy when using Velocity modern forwarding.
//
// The relay uses loginInboundConn to send LoginPluginMessages to the client and
// receive responses via consumer callbacks. The client's read loop goroutine
// processes responses via the auth session handler's HandlePacket.
type modernForgeLoginRelay struct {
	clientLogin *loginInboundConn
	player      *connectedPlayer

	// pendingLoginSuccess is the ServerLoginSuccess packet to send to the client
	// after the FML handshake completes.
	pendingLoginSuccess *packet.ServerLoginSuccess

	mu              sync.Mutex
	cachedExchanges []forgeLoginExchange
}

// forgeLoginExchange records a single FML LoginPluginMessage exchange
// between the backend and client, for replay during server switch.
type forgeLoginExchange struct {
	Channel  string
	Request  []byte // data sent by backend
	Response []byte // data from client (nil if client rejected)
	Success  bool
}

func newModernForgeLoginRelay(
	clientLogin *loginInboundConn,
	player *connectedPlayer,
	pendingLoginSuccess *packet.ServerLoginSuccess,
) *modernForgeLoginRelay {
	return &modernForgeLoginRelay{
		clientLogin:         clientLogin,
		player:              player,
		pendingLoginSuccess: pendingLoginSuccess,
	}
}

// relayToClient forwards a backend LoginPluginMessage to the client via the
// login plugin message mechanism. The consumer callback will forward the
// client's response back to the backend.
func (r *modernForgeLoginRelay) relayToClient(
	backendConn netmc.MinecraftConn,
	msg *packet.LoginPluginMessage,
) error {
	identifier, err := message.ChannelIdentifierFrom(msg.Channel)
	if err != nil {
		return err
	}

	data := msg.Data
	if len(data) == 0 {
		// SendLoginPluginMessage requires non-empty data.
		data = []byte{0}
	}

	consumer := &forgeRelayConsumer{
		relay:        r,
		backendConn:  backendConn,
		backendMsgID: msg.ID,
		channel:      msg.Channel,
		requestData:  msg.Data,
	}
	return r.clientLogin.SendLoginPluginMessage(identifier, data, consumer)
}

// complete sends the pending ServerLoginSuccess to the client,
// completing the delayed login.
func (r *modernForgeLoginRelay) complete() error {
	return r.player.WritePacket(r.pendingLoginSuccess)
}

// exchanges returns a copy of the cached FML exchanges for server switch replay.
func (r *modernForgeLoginRelay) exchanges() []forgeLoginExchange {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]forgeLoginExchange, len(r.cachedExchanges))
	copy(out, r.cachedExchanges)
	return out
}

// forgeRelayConsumer forwards a client's LoginPluginResponse back to the
// backend server. It also caches the exchange for future server switch replay.
type forgeRelayConsumer struct {
	relay        *modernForgeLoginRelay
	backendConn  netmc.MinecraftConn
	backendMsgID int
	channel      string
	requestData  []byte
}

func (c *forgeRelayConsumer) OnMessageResponse(responseBody []byte) error {
	// Cache the exchange for server switch replay.
	c.relay.mu.Lock()
	c.relay.cachedExchanges = append(c.relay.cachedExchanges, forgeLoginExchange{
		Channel:  c.channel,
		Request:  c.requestData,
		Response: responseBody,
		Success:  responseBody != nil,
	})
	c.relay.mu.Unlock()

	// Forward the response to the backend.
	return c.backendConn.WritePacket(&packet.LoginPluginResponse{
		ID:      c.backendMsgID,
		Success: responseBody != nil,
		Data:    responseBody,
	})
}

// modernForgeReplayRelay replays cached FML LoginPluginMessage responses
// during a server switch when the client is already in PLAY state and
// cannot participate in a new LOGIN-phase FML handshake.
type modernForgeReplayRelay struct {
	cachedExchanges []forgeLoginExchange
	mu              sync.Mutex
	replayIndex     int
}

func newModernForgeReplayRelay(cached []forgeLoginExchange) *modernForgeReplayRelay {
	return &modernForgeReplayRelay{
		cachedExchanges: cached,
	}
}

// replayResponse sends the next cached response to the backend.
// If no more cached responses are available, sends Success=false.
func (r *modernForgeReplayRelay) replayResponse(
	backendMsgID int,
	backendConn netmc.MinecraftConn,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.replayIndex >= len(r.cachedExchanges) {
		return backendConn.WritePacket(&packet.LoginPluginResponse{
			ID:      backendMsgID,
			Success: false,
		})
	}

	exchange := r.cachedExchanges[r.replayIndex]
	r.replayIndex++

	return backendConn.WritePacket(&packet.LoginPluginResponse{
		ID:      backendMsgID,
		Success: exchange.Success,
		Data:    exchange.Response,
	})
}
