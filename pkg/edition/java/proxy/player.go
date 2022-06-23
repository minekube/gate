package proxy

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.uber.org/atomic"

	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/modinfo"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/title"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/edition/java/proxy/player"
	"go.minekube.com/gate/pkg/util/permission"
	"go.minekube.com/gate/pkg/util/sets"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Player is a connected Minecraft player.
type Player interface {
	Inbound
	command.Source
	message.ChannelMessageSource
	message.ChannelMessageSink
	crypto.KeyIdentifiable

	ID() uuid.UUID    // The Minecraft ID of the player.
	Username() string // The username of the player.
	// CurrentServer returns the current server connection of the player.
	CurrentServer() ServerConnection // May be nil, if there is no backend server connection!
	Ping() time.Duration             // The player's ping or -1 if currently unknown.
	OnlineMode() bool                // Whether the player was authenticated with Mojang's session servers.
	// CreateConnectionRequest creates a connection request to begin switching the backend server.
	CreateConnectionRequest(target RegisteredServer) ConnectionRequest
	GameProfile() profile.GameProfile // Returns the player's game profile.
	Settings() player.Settings        // The player's client settings. Returns player.DefaultSettings if unknown.
	// Disconnect disconnects the player with a reason.
	// Once called, further interface calls to this player become undefined.
	Disconnect(reason component.Component)
	// SpoofChatInput sends chats input onto the player's current server as if
	// they typed it into the client chat box.
	SpoofChatInput(input string) error
	// SendResourcePack sends the specified resource pack from url to the user. If at all possible,
	// send the resource pack with a sha1 hash using SendResourcePackWithHash. To monitor the status
	// of the sent resource pack, subscribe to PlayerResourcePackStatusEvent.
	SendResourcePack(url string) error
	// SendResourcePackWithHash sends the specified resource pack from url to the user,
	// using the specified 20-byte SHA-1 hash of the resource pack file. To monitor the
	// status of the sent resource pack, subscribe to PlayerResourcePackStatusEvent.
	SendResourcePackWithHash(url string, sha1Hash []byte) error
	// SendActionBar sends an action bar to the player.
	SendActionBar(msg component.Component) error
	// SendMessageWith sends a chat message with optional modifications.
	SendMessageWith(msg component.Component, opts ...MessageOption) error
	TabList() tablist.TabList // Returns the player's tab list.
	// TODO add title and more
}

type connectedPlayer struct {
	*minecraftConn
	log         logr.Logger
	virtualHost net.Addr
	onlineMode  bool
	profile     *profile.GameProfile
	ping        atomic.Duration
	permFunc    permission.Func
	playerKey   crypto.IdentifiedKey // 1.19+

	// This field is true if this connection is being disconnected
	// due to another connection logging in with the same GameProfile.
	disconnectDueToDuplicateConnection atomic.Bool

	pluginChannelsMu sync.RWMutex // Protects following field
	pluginChannels   sets.String  // Known plugin channels

	tabList tablist.TabList // Player's tab list

	mu               sync.RWMutex // Protects following fields
	connectedServer_ *serverConnection
	connInFlight     *serverConnection
	settings         player.Settings
	modInfo          *modinfo.ModInfo
	connPhase        clientConnectionPhase

	serversToTry []string // names of servers to try if we got disconnected from previous
	tryIndex     int
}

var _ Player = (*connectedPlayer)(nil)

func newConnectedPlayer(
	conn *minecraftConn,
	profile *profile.GameProfile,
	virtualHost net.Addr,
	onlineMode bool,
	playerKey crypto.IdentifiedKey, // nil-able
) *connectedPlayer {
	ping := atomic.Duration{}
	ping.Store(-1)

	var tabList tablist.TabList
	if conn.protocol.GreaterEqual(version.Minecraft_1_8) {
		tabList = tablist.New(conn, conn.protocol, &tabListPlayerKeyStore{p: conn.proxy})
	} else {
		tabList = tablist.NewLegacy(conn, conn.protocol)
	}

	return &connectedPlayer{
		minecraftConn: conn,
		log: conn.log.WithName("player").WithValues(
			"name", profile.Name, "id", profile.ID),
		profile:        profile,
		virtualHost:    virtualHost,
		onlineMode:     onlineMode,
		pluginChannels: sets.NewString(), // Should we limit the size to 1024 channels?
		connPhase:      conn.Type().initialClientPhase(),
		ping:           ping,
		tabList:        tabList,
		permFunc:       func(string) permission.TriState { return permission.Undefined },
		playerKey:      playerKey,
	}
}

type tabListPlayerKeyStore struct{ p *Proxy }

var _ tablist.PlayerKey = (*tabListPlayerKeyStore)(nil)

func (t *tabListPlayerKeyStore) PlayerKey(playerID uuid.UUID) crypto.IdentifiedKey {
	return t.p.player(playerID).playerKey
}

func (p *connectedPlayer) IdentifiedKey() crypto.IdentifiedKey { return p.playerKey }

func (p *connectedPlayer) connectionInFlight() *serverConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connInFlight
}

func (p *connectedPlayer) phase() clientConnectionPhase {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connPhase
}

func (p *connectedPlayer) HasPermission(permission string) bool {
	return p.PermissionValue(permission).Bool()
}

func (p *connectedPlayer) PermissionValue(permission string) permission.TriState {
	return p.permFunc(permission)
}

func (p *connectedPlayer) Ping() time.Duration {
	return p.ping.Load()
}

func (p *connectedPlayer) OnlineMode() bool {
	return p.onlineMode
}

func (p *connectedPlayer) GameProfile() profile.GameProfile {
	return *p.profile
}

func (p *connectedPlayer) TabList() tablist.TabList { return p.tabList }

var (
	ErrNoBackendConnection = errors.New("player has no backend server connection yet")
	ErrTooLongChatMessage  = errors.New("server bound chat message can not exceed 256 characters")
)

func (p *connectedPlayer) SpoofChatInput(input string) error {
	if len(input) > packet.MaxServerBoundMessageLength {
		return ErrTooLongChatMessage
	}

	serverMc, ok := p.ensureBackendConnection()
	if !ok {
		return ErrNoBackendConnection
	}
	return serverMc.WritePacket(&packet.LegacyChat{
		Message: input,
		Type:    packet.ChatMessageType,
	})
}

func (p *connectedPlayer) ensureBackendConnection() (*minecraftConn, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.connectedServer_ == nil {
		// Player has no backend connection.
		return nil, false
	}
	serverMc := p.connectedServer_.conn()
	if serverMc == nil {
		// Player's backend connection is not yet connected to a server.
		return nil, false
	}
	return serverMc, true
}

func (p *connectedPlayer) SendResourcePack(url string) error {
	return p.WritePacket(&packet.ResourcePackRequest{
		Url:  url,
		Hash: "",
	})
}

func (p *connectedPlayer) SendResourcePackWithHash(url string, sha1Hash []byte) error {
	if len(sha1Hash) != 20 {
		return errors.New("hash length must be 20")
	}
	return p.WritePacket(&packet.ResourcePackRequest{
		Url:  url,
		Hash: hex.EncodeToString(sha1Hash),
	})
}

func (p *connectedPlayer) VirtualHost() net.Addr {
	return p.virtualHost
}

func (p *connectedPlayer) Active() bool {
	return !p.minecraftConn.Closed()
}

// MessageOption is an option for Player.SendMessageWith.
type MessageOption func(c *packet.LegacyChat)

// MessageWithSender modifies the sender identity of the chat message.
func MessageWithSender(id uuid.UUID) MessageOption {
	return func(c *packet.LegacyChat) { c.Sender = id }
}

// MessageType is a chat message type.
type MessageType uint8

// Chat message types.
const (
	// ChatMessageType is a standard chat message.
	ChatMessageType MessageType = iota
	// SystemMessageType is a system chat message.
	// e.g. client is willing to accept messages from commands,
	// but does not want general chat from other players.
	SystemMessageType
)

// MessageWithType modifies chat message type.
func MessageWithType(t MessageType) MessageOption {
	return func(c *packet.LegacyChat) {
		if t == SystemMessageType {
			c.Type = packet.SystemMessageType
		} else {
			c.Type = packet.ChatMessageType
		}
	}
}

func (p *connectedPlayer) SendMessage(msg component.Component) error { return p.SendMessageWith(msg) }
func (p *connectedPlayer) SendMessageWith(msg component.Component, opts ...MessageOption) error {
	if msg == nil {
		return nil // skip nil message
	}
	m := new(strings.Builder)
	if err := util.JsonCodec(p.Protocol()).Marshal(m, msg); err != nil {
		return err
	}
	chat := &packet.LegacyChat{
		Message: m.String(),
		Type:    packet.ChatMessageType,
		Sender:  uuid.Nil,
	}
	for _, o := range opts {
		o(chat)
	}
	return p.WritePacket(chat)
}

var legacyJsonCodec = &legacy.Legacy{}

func (p *connectedPlayer) SendActionBar(msg component.Component) error {
	if msg == nil {
		return nil // skip nil message
	}
	protocol := p.Protocol()
	if protocol.GreaterEqual(version.Minecraft_1_11) {
		// Use the title packet instead.
		pkt, err := title.New(protocol, &title.Builder{
			Action:    title.SetActionBar,
			Component: msg,
		})
		if err != nil {
			return err
		}
		return p.WritePacket(pkt)
	}

	// Due to issues with action bar packets, we'll need to convert the text message into a
	// legacy message and then put the legacy text into a component... (╯°□°)╯︵ ┻━┻!
	b := new(strings.Builder)
	if err := legacyJsonCodec.Marshal(b, msg); err != nil {
		return err
	}
	m, err := json.Marshal(map[string]string{"text": b.String()})
	if err != nil {
		return err
	}
	return p.WritePacket(&packet.LegacyChat{ // TODO 1.19 chat
		Message: string(m),
		Type:    packet.GameInfoMessageType,
		Sender:  uuid.Nil,
	})
}

func (p *connectedPlayer) SendPluginMessage(identifier message.ChannelIdentifier, data []byte) error {
	return p.WritePacket(&plugin.Message{
		Channel: identifier.ID(),
		Data:    data,
	})
}

// TODO add header/footer, title & boss bar methods

// Finds another server to attempt to log into, if we were unexpectedly disconnected from the server.
// current is the current server of the player is on, so we skip this server and not connect to it.
// current can be nil if there is no current server.
// MAY RETURN NIL if no next server available!
func (p *connectedPlayer) nextServerToTry(current RegisteredServer) RegisteredServer {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.serversToTry) == 0 {
		p.serversToTry = p.proxy.Config().ForcedHosts[p.virtualHost.String()]
	}
	if len(p.serversToTry) == 0 {
		connOrder := p.proxy.Config().Try
		if len(connOrder) == 0 {
			return nil
		} else {
			p.serversToTry = connOrder
		}
	}

	sameName := func(rs RegisteredServer, name string) bool {
		return rs.ServerInfo().Name() == name
	}

	for i := p.tryIndex; i < len(p.serversToTry); i++ {
		toTry := p.serversToTry[i]
		if (p.connectedServer_ != nil && sameName(p.connectedServer_.Server(), toTry)) ||
			(p.connInFlight != nil && sameName(p.connInFlight.Server(), toTry)) ||
			(current != nil && sameName(current, toTry)) {
			continue
		}

		p.tryIndex = i
		if s := p.proxy.Server(toTry); s != nil {
			return s
		}
	}
	return nil
}

// player's connection is closed at this point,
// now need to disconnect backend server connection, if any.
func (p *connectedPlayer) teardown() {
	p.mu.RLock()
	connInFlight := p.connInFlight
	connectedServer := p.connectedServer_
	p.mu.RUnlock()
	if connInFlight != nil {
		connInFlight.disconnect()
	}
	if connectedServer != nil {
		connectedServer.disconnect()
	}

	var status LoginStatus
	if p.proxy.unregisterConnection(p) {
		if p.disconnectDueToDuplicateConnection.Load() {
			status = ConflictingLoginStatus
		} else {
			status = SuccessfulLoginStatus
		}
	} else {
		if p.knownDisconnect.Load() {
			status = CanceledByProxyLoginStatus
		} else {
			status = CanceledByUserLoginStatus
		}
	}
	p.proxy.event.Fire(&DisconnectEvent{
		player:      p,
		loginStatus: status,
	})
}

// may be nil!
func (p *connectedPlayer) CurrentServer() ServerConnection {
	if cs := p.connectedServer(); cs != nil {
		return cs
	}
	// We must return an explicit nil, not a (*serverConnection)(nil).
	return nil
}

func (p *connectedPlayer) connectedServer() *serverConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connectedServer_
}

func (p *connectedPlayer) Username() string { return p.profile.Name }

func (p *connectedPlayer) ID() uuid.UUID { return p.profile.ID }

func (p *connectedPlayer) Disconnect(reason component.Component) {
	if !p.Active() {
		return
	}

	var r string
	b := new(strings.Builder)
	if (&legacy.Legacy{}).Marshal(b, reason) == nil {
		r = b.String()
	}

	if p.closeWith(packet.DisconnectWithProtocol(reason, p.Protocol())) == nil {
		p.log.Info("Player has been disconnected", "reason", r)
	}
}

func (p *connectedPlayer) String() string { return p.profile.Name }

func (p *connectedPlayer) sendLegacyForgeHandshakeResetPacket() {
	p.phase().resetConnectionPhase(p)
}

func (p *connectedPlayer) setPhase(phase *legacyForgeHandshakeClientPhase) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connPhase = phase
}

// may return nil
func (p *connectedPlayer) ModInfo() *modinfo.ModInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.modInfo
}

func (p *connectedPlayer) setModInfo(info *modinfo.ModInfo) {
	p.mu.Lock()
	p.modInfo = info
	p.mu.Unlock()

	if info != nil {
		p.proxy.Event().Fire(&PlayerModInfoEvent{
			player:  p,
			modInfo: *info,
		})
	}
}

// NOTE: the returned set is not goroutine-safe and must not be modified,
// it is only for reading!!!
func (p *connectedPlayer) knownChannels() sets.String {
	p.pluginChannelsMu.RLock()
	defer p.pluginChannelsMu.RUnlock()
	return p.pluginChannels
}

// runs fn while pluginChannels is locked. Used for modifying channel set.
func (p *connectedPlayer) lockedKnownChannels(fn func(knownChannels sets.String)) {
	p.pluginChannelsMu.RLock()
	defer p.pluginChannelsMu.RUnlock()
	fn(p.pluginChannels)
}

func (p *connectedPlayer) setConnectedServer(conn *serverConnection) {
	p.mu.Lock()
	p.connectedServer_ = conn
	p.tryIndex = 0 // reset since we got connected to a server
	if conn == p.connInFlight {
		p.connInFlight = nil
	}
	p.mu.Unlock()
}

func (p *connectedPlayer) setSettings(settings *packet.ClientSettings) {
	wrapped := player.NewSettings(settings)
	p.mu.Lock()
	p.settings = wrapped
	p.mu.Unlock()

	p.proxy.Event().Fire(&PlayerSettingsChangedEvent{
		player:   p,
		settings: wrapped,
	})
}

func (p *connectedPlayer) Closed() <-chan struct{} {
	return p.minecraftConn.closed
}

// Settings returns the players client settings.
// If not known already, returns player.DefaultSettings.
func (p *connectedPlayer) Settings() player.Settings {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.settings != nil {
		return p.settings
	}
	return player.DefaultSettings
}

// returns a new player context that is canceled when:
//  - connection disconnects
//  - parent was canceled
func (c *minecraftConn) newContext(parent context.Context) (ctx context.Context, cancel func()) {
	ctx, cancel = context.WithCancel(parent)
	go func() {
		select {
		case <-ctx.Done():
		case <-c.closed: // TODO use context.Context all along so we don't need to start a new goroutine
			cancel()
		}
	}()
	return ctx, cancel
}

func randomUint64() uint64 {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf) // Always succeeds, no need to check error
	return binary.LittleEndian.Uint64(buf)
}
