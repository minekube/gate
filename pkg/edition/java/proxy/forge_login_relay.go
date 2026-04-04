package proxy

import (
	"fmt"
	"sync"

	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
)

// ForgeLoginWrapperChannel is the Forge login wrapper channel used for FML2/FML3
// mod negotiation during the LOGIN phase (Minecraft 1.13-1.20.1).
const ForgeLoginWrapperChannel = "fml:loginwrapper"

// modernForgeLoginRelay relays LoginPluginMessages between a backend Forge server
// and a client during the LOGIN phase. This enables Forge 1.13-1.20.1 mod negotiation
// through the proxy when using Velocity modern forwarding.
//
// The relay works synchronously: the backend goroutine writes LoginPluginMessages to
// the client connection and reads LoginPluginResponses directly. This is safe because
// the client's read loop goroutine is blocked in connectToInitialServer (application-
// level blocking), not in Decode, so the decoder mutex is not held and no concurrent
// reads occur.
type modernForgeLoginRelay struct {
	player *connectedPlayer

	// pendingLoginSuccess is the ServerLoginSuccess packet to send to the client
	// after the FML handshake completes.
	pendingLoginSuccess *packet.ServerLoginSuccess

	// nextClientID tracks the next message ID to use when sending to the client.
	// Backend and client use independent ID spaces.
	nextClientID int

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
	player *connectedPlayer,
	pendingLoginSuccess *packet.ServerLoginSuccess,
) *modernForgeLoginRelay {
	return &modernForgeLoginRelay{
		player:              player,
		pendingLoginSuccess: pendingLoginSuccess,
	}
}

// relayToClient forwards a backend LoginPluginMessage to the client and reads
// the client's response synchronously. The response is cached for future server
// switch replay and forwarded to the backend.
//
// This method reads directly from the client connection. It is safe to call from
// the backend goroutine because the client's read loop is blocked in
// connectToInitialServer (not in Decode), so the decoder mutex is free.
func (r *modernForgeLoginRelay) relayToClient(
	backendConn netmc.MinecraftConn,
	msg *packet.LoginPluginMessage,
) error {
	// Send LoginPluginMessage to client with our own ID
	clientID := r.nextClientID
	r.nextClientID++

	if err := r.player.WritePacket(&packet.LoginPluginMessage{
		ID:      clientID,
		Channel: msg.Channel,
		Data:    msg.Data,
	}); err != nil {
		return fmt.Errorf("error sending forge message to client: %w", err)
	}

	// Read LoginPluginResponse directly from the client connection.
	// The client's read loop goroutine is blocked in connectToInitialServer,
	// so we are the only reader on this connection.
	pc, err := r.player.MinecraftConn.Reader().ReadPacket()
	if err != nil {
		return fmt.Errorf("error reading forge response from client: %w", err)
	}

	resp, ok := pc.Packet.(*packet.LoginPluginResponse)
	if !ok {
		return fmt.Errorf("expected LoginPluginResponse from client, got %T", pc.Packet)
	}

	// Cache the exchange for server switch replay.
	r.mu.Lock()
	r.cachedExchanges = append(r.cachedExchanges, forgeLoginExchange{
		Channel:  msg.Channel,
		Request:  msg.Data,
		Response: resp.Data,
		Success:  resp.Success,
	})
	r.mu.Unlock()

	// Forward the response to the backend with the original backend message ID.
	return backendConn.WritePacket(&packet.LoginPluginResponse{
		ID:      msg.ID,
		Success: resp.Success,
		Data:    resp.Data,
	})
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
