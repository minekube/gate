package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robinbraemer/event"

	cfgpacket "go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"go.minekube.com/gate/pkg/edition/java/proxy/internal/resourcepack"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/future"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/sets"

	"github.com/go-logr/logr"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.uber.org/atomic"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/lite"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	internaltablist "go.minekube.com/gate/pkg/internal/tablist"

	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/title"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/edition/java/proxy/player"
	"go.minekube.com/gate/pkg/util/permission"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Player is a connected Minecraft player.
type Player interface { // TODO convert to struct(?) bc this is a lot of methods and only *connectedPlayer implements it
	Inbound
	netmc.PacketWriter
	command.Source
	message.ChannelMessageSource
	// ChannelMessageSink sends a plugin message to the player's client.
	//
	// Note that this method does not send a plugin message to the server the player
	// is connected to. You should only use this method if you are trying to communicate
	// with a mod that is installed on the player's client.
	// To send a plugin message to the server from the player,
	// you should use CurrentServer(), check for non-nil and then currSrv.SendPluginMessage().
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
	// SendResourcePack sends the specified resource pack from url to the user.
	// If at all possible, specify an 20-byte SHA-1 hash of the resource pack file.
	// To monitor the status of the sent resource pack, subscribe to PlayerResourcePackStatusEvent.
	SendResourcePack(info ResourcePackInfo) error
	// AppliedResourcePacks returns all applied resource packs that were applied to the player.
	AppliedResourcePacks() []*ResourcePackInfo
	// PendingResourcePacks returns all pending resource packs that are currently being sent to the player.
	PendingResourcePacks() []*ResourcePackInfo
	// SendActionBar sends an action bar to the player.
	SendActionBar(msg component.Component) error
	// TabList returns the player's tab list.
	// Used for modifying the player's tab list and header/footer.
	TabList() tablist.TabList
	ClientBrand() string // Returns the player's client brand. Empty if unspecified.
	// TransferToHost transfers the player to the specified host.
	// The host should be in the format of "host:port" or just "host" in which case the port defaults to 25565.
	// If the player is from a version lower than 1.20.5, this method will return ErrTransferUnsupportedClientProtocol.
	TransferToHost(addr string) error

	// AppliedResourcePack returns the resource pack that was applied to the player.
	// Returns nil if no resource pack was applied.
	//
	// Deprecated: Use AppliedResourcePacks instead.
	AppliedResourcePack() *ResourcePackInfo
	// PendingResourcePack returns the resource pack that is currently being sent to the player.
	// Returns nil if no resource pack is being sent.
	//
	// Deprecated: Use PendingResourcePacks instead.
	PendingResourcePack() *ResourcePackInfo

	// Context retrieves the player's context.
	// This context is invalidated when the player's connection is closed.
	//
	// It is beneficial for managing timeouts, cancellations, and other
	// operations that depend on the player, allowing them to run in the
	// background only while the player connection remains active.
	Context() context.Context

	// Looking for more methods?
	//
	// Use the dedicated packages:
	//  - https://pkg.go.dev/go.minekube.com/gate/pkg/edition/java/bossbar
	//  - https://pkg.go.dev/go.minekube.com/gate/pkg/edition/java/title
	//  - https://pkg.go.dev/go.minekube.com/gate/pkg/edition/java/cookie
	//  - https://pkg.go.dev/go.minekube.com/gate/pkg/edition/java/proxy/tablist
}

type connectedPlayer struct {
	netmc.MinecraftConn
	*sessionHandlerDeps

	log                 logr.Logger
	virtualHost         net.Addr
	onlineMode          bool
	profile             *profile.GameProfile
	ping                atomic.Duration
	permFunc            permission.Func
	playerKey           crypto.IdentifiedKey // 1.19+
	resourcePackHandler resourcepack.Handler
	bundleHandler       *resourcepack.BundleDelimiterHandler
	chatQueue           *chatQueue
	handshakeIntent     packet.HandshakeIntent
	// This field is true if this connection is being disconnected
	// due to another connection logging in with the same GameProfile.
	disconnectDueToDuplicateConnection atomic.Bool
	clientsideChannels                 *sets.CappedSet[string]
	pendingConfigurationSwitch         bool

	tabList internaltablist.InternalTabList // Player's tab list

	mu                   sync.RWMutex // Protects following fields
	connectedServer_     *serverConnection
	connInFlight         *serverConnection
	settings             player.Settings
	clientSettingsPacket *packet.ClientSettings
	modInfo              *modinfo.ModInfo
	connPhase            phase.ClientConnectionPhase

	clientBrand string // may be empty

	serversToTry []string // names of servers to try if we got disconnected from previous
	tryIndex     int
}

var _ Player = (*connectedPlayer)(nil)

const maxClientsidePluginChannels = 1024

func newConnectedPlayer(
	conn netmc.MinecraftConn,
	profile *profile.GameProfile,
	virtualHost net.Addr,
	handshakeIntent packet.HandshakeIntent,
	onlineMode bool,
	playerKey crypto.IdentifiedKey, // nil-able
	sessionHandlerDeps *sessionHandlerDeps,
) *connectedPlayer {
	var ping atomic.Duration
	ping.Store(-1)

	p := &connectedPlayer{
		sessionHandlerDeps: sessionHandlerDeps,
		MinecraftConn:      conn,
		log: logr.FromContextOrDiscard(conn.Context()).WithName("player").WithValues(
			"name", profile.Name, "id", profile.ID),
		profile:            profile,
		virtualHost:        virtualHost,
		handshakeIntent:    handshakeIntent,
		clientsideChannels: sets.NewCappedSet[string](maxClientsidePluginChannels),
		onlineMode:         onlineMode,
		connPhase:          conn.Type().InitialClientPhase(),
		ping:               ping,
		permFunc:           func(string) permission.TriState { return permission.Undefined },
		playerKey:          playerKey,
	}
	p.resourcePackHandler = resourcepack.NewHandler(p, p.eventMgr)
	p.bundleHandler = &resourcepack.BundleDelimiterHandler{Player: p}
	p.chatQueue = newChatQueue(p)
	p.tabList = internaltablist.New(p)
	return p
}

func (p *connectedPlayer) IdentifiedKey() crypto.IdentifiedKey { return p.playerKey }

func (p *connectedPlayer) connectionInFlight() *serverConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connInFlight
}

func (p *connectedPlayer) connectionInFlightOrConnectedServer() *serverConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.connInFlight != nil {
		return p.connInFlight
	}
	return p.connectedServer_
}

func (p *connectedPlayer) phase() phase.ClientConnectionPhase {
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
	if len(input) > chat.MaxServerBoundMessageLength {
		return ErrTooLongChatMessage
	}

	msg := &chat.Builder{
		Protocol: p.Protocol(),
		Sender:   p.ID(),
		Message:  input,
	}

	if p.Protocol().GreaterEqual(version.Minecraft_1_19) {
		p.chatQueue.QueuePacketWithFunction(func(chatState *ChatState) proto.Packet {
			msg.Timestamp = chatState.LastTimestamp()
			msg.LastSeenMessages = chatState.CreateLastSeen()
			return msg.ToServer()
		})
		return nil
	}

	serverMc, ok := p.ensureBackendConnection()
	if !ok {
		return ErrNoBackendConnection
	}
	return serverMc.WritePacket(msg.ToServer())
}

func (p *connectedPlayer) ensureBackendConnection() (netmc.MinecraftConn, bool) {
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

func (p *connectedPlayer) SendResourcePack(info ResourcePackInfo) error {
	if err := p.resourcePackHandler.CheckAlreadyAppliedPack(info.Hash); err != nil {
		return err
	}
	if !p.Protocol().GreaterEqual(version.Minecraft_1_8) {
		return nil
	}
	return p.resourcePackHandler.QueueResourcePack(&info)
}

//nolint:unused
func (p *connectedPlayer) clearResourcePacks() error {
	defer p.resourcePackHandler.ClearAppliedResourcePacks()
	if p.Protocol().GreaterEqual(version.Minecraft_1_20_3) {
		return p.WritePacket(&packet.RemoveResourcePack{})
	}
	return nil
}

//nolint:unused
func (p *connectedPlayer) removeResourcePacks(ids ...uuid.UUID) error {
	if !p.Protocol().GreaterEqual(version.Minecraft_1_20_3) {
		return nil
	}
	for _, id := range ids {
		if p.resourcePackHandler.Remove(id) {
			err := p.WritePacket(&packet.RemoveResourcePack{ID: id})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// ResourcePackInfo is resource-pack options for Player.SendResourcePack.
type ResourcePackInfo = resourcepack.Info

// ResourcePackOrigin represents the origin of the resource-pack.
type ResourcePackOrigin = resourcepack.Origin

const (
	DownstreamServerResourcePackOrigin = resourcepack.DownstreamServerOrigin
	PluginOnProxyResourcePackOrigin    = resourcepack.PluginOnProxyOrigin
)

// AppliedResourcePack returns the resource-pack that was applied and accepted by the player.
// It returns nil if there is no applied resource-pack.
func (p *connectedPlayer) AppliedResourcePack() *ResourcePackInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.resourcePackHandler.FirstAppliedPack()
}

// AppliedResourcePacks returns all applied resource-packs that were applied and accepted by the player.
func (p *connectedPlayer) AppliedResourcePacks() []*ResourcePackInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.resourcePackHandler.AppliedResourcePacks()
}

// PendingResourcePack returns the resource-pack that is currently being sent to the player.
// It returns nil if there is no pending resource-pack.
func (p *connectedPlayer) PendingResourcePack() *ResourcePackInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.resourcePackHandler.FirstPendingPack()
}

// PendingResourcePacks returns all pending resource-packs that are currently being sent to the player.
func (p *connectedPlayer) PendingResourcePacks() []*ResourcePackInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.resourcePackHandler.PendingResourcePacks()
}

func (p *connectedPlayer) VirtualHost() net.Addr {
	return p.virtualHost
}

func (p *connectedPlayer) Active() bool {
	return !netmc.Closed(p.MinecraftConn)
}

// WithMessageSender modifies the sender identity of the chat message.
func WithMessageSender(id uuid.UUID) command.MessageOption {
	return messageApplyOption(func(o any) {
		if b, ok := o.(*chat.Builder); ok {
			b.Sender = id
		}
	})
}

// MessageType is a chat message type.
type MessageType = chat.MessageType

// Chat message types.
const (
	// ChatMessageType is a standard chat message and
	// lets the chat message appear in the client's HUD.
	// These messages can be filtered out by the client's settings.
	ChatMessageType = chat.ChatMessageType
	// SystemMessageType is a system chat message.
	// e.g. client is willing to accept messages from commands,
	// but does not want general chat from other players.
	// It lets the chat message appear in the client's HUD and can't be dismissed.
	SystemMessageType = chat.SystemMessageType
	// GameInfoMessageType lets the chat message appear above the player's main HUD.
	// This text format doesn't support many component features, such as hover events.
	GameInfoMessageType = chat.GameInfoMessageType
)

// WithMessageType modifies chat message type.
func WithMessageType(t MessageType) command.MessageOption {
	return messageApplyOption(func(o any) {
		if b, ok := o.(*chat.Builder); ok {
			if t != ChatMessageType {
				t = SystemMessageType
			}
			b.Type = t
		}
	})
}

type messageApplyOption func(o any)

func (a messageApplyOption) Apply(o any) { a(o) }

func (p *connectedPlayer) SendMessage(msg component.Component, opts ...command.MessageOption) error {
	if msg == nil {
		return nil // skip nil message
	}
	b := chat.Builder{
		Protocol:  p.Protocol(),
		Type:      ChatMessageType,
		Sender:    p.ID(),
		Component: msg,
	}
	for _, o := range opts {
		o.Apply(b)
	}
	return p.WritePacket(b.ToClient())
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
			Component: *chat.FromComponent(msg),
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
	return p.WritePacket(&chat.LegacyChat{
		Message: string(m),
		Type:    chat.GameInfoMessageType,
		Sender:  uuid.Nil,
	})
}

func (p *connectedPlayer) SendPluginMessage(identifier message.ChannelIdentifier, data []byte) error {
	return p.WritePacket(&plugin.Message{
		Channel: identifier.ID(),
		Data:    data,
	})
}

// Finds another server to attempt to log into, if we were unexpectedly disconnected from the server.
// current is the current server of the player is on, so we skip this server and not connect to it.
// current can be nil if there is no current server.
// MAY RETURN NIL if no next server available!
func (p *connectedPlayer) nextServerToTry(current RegisteredServer) RegisteredServer {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.serversToTry) == 0 {
		// Extract hostname from virtual host and convert to lowercase
		virtualHostStr := p.getVirtualHostname()
		p.serversToTry = p.config().ForcedHosts[virtualHostStr]
	}
	if len(p.serversToTry) == 0 {
		connOrder := p.config().Try
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

// getVirtualHostname extracts the hostname from the virtual host address and converts it to lowercase.
func (p *connectedPlayer) getVirtualHostname() string {
	if p.virtualHost == nil {
		return ""
	}
	
	// Use Gate's existing utility functions to clean the virtual host
	// 1. Clear virtual host (removes forge separators, TCPShield separators, etc.)
	// 2. Extract hostname (removes port)
	// 3. Convert to lowercase for consistent matching
	virtualHostStr := p.virtualHost.String()
	cleanedHost := lite.ClearVirtualHost(virtualHostStr)
	hostname := netutil.HostStr(cleanedHost)
	
	return strings.ToLower(hostname)
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
	if p.registrar.unregisterConnection(p) {
		if p.disconnectDueToDuplicateConnection.Load() {
			status = ConflictingLoginStatus
		} else {
			status = SuccessfulLoginStatus
		}
	} else {
		if netmc.KnownDisconnect(p) {
			status = CanceledByProxyLoginStatus
		} else {
			status = CanceledByUserLoginStatus
		}
	}
	p.eventMgr.Fire(&DisconnectEvent{
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

	if netmc.CloseWith(p, packet.NewDisconnect(reason, p.Protocol(), p.State().State)) == nil {
		p.log.Info("player has been disconnected", "reason", r)
	}
}

func (p *connectedPlayer) String() string { return p.profile.Name }

func (p *connectedPlayer) SendLegacyForgeHandshakeResetPacket() {
	p.phase().ResetConnectionPhase(p, p)
}

func (p *connectedPlayer) SetPhase(phase phase.ClientConnectionPhase) {
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

func (p *connectedPlayer) SetModInfo(info *modinfo.ModInfo) {
	p.mu.Lock()
	p.modInfo = info
	p.mu.Unlock()

	if info != nil {
		p.eventMgr.Fire(&PlayerModInfoEvent{
			player:  p,
			modInfo: *info,
		})
	}
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

func (p *connectedPlayer) setClientSettings(settings *packet.ClientSettings) {
	wrapped := player.NewSettings(settings)
	p.mu.Lock()
	p.settings = wrapped
	p.clientSettingsPacket = settings
	p.mu.Unlock()

	p.eventMgr.Fire(&PlayerSettingsChangedEvent{
		player:   p,
		settings: wrapped,
	})
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

// ClientSettingsPacket returns the last known client settings packet.
// If not known already, returns nil.
func (p *connectedPlayer) ClientSettingsPacket() *packet.ClientSettings {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clientSettingsPacket
}

func (p *connectedPlayer) config() *config.Config {
	return p.configProvider.config()
}

// switchToConfigState switches the connection of the client into config state.
func (p *connectedPlayer) switchToConfigState() {
	if err := p.BufferPacket(new(cfgpacket.StartUpdate)); err != nil {
		p.log.Error(err, "error writing config packet")
	}

	p.pendingConfigurationSwitch = true
	p.MinecraftConn.Writer().SetState(state.Config)
	// Make sure we don't send any play packets to the player after update start
	p.MinecraftConn.EnablePlayPacketQueue()

	_ = p.Flush() // Trigger switch finally
}

func (p *connectedPlayer) ClientBrand() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clientBrand
}

// setClientBrand sets the client brand of the player.
func (p *connectedPlayer) setClientBrand(brand string) {
	p.mu.Lock()
	p.clientBrand = brand
	p.mu.Unlock()
}

var ErrTransferUnsupportedClientProtocol = errors.New("player version must be 1.20.5 to be able to transfer to another host")

func (p *connectedPlayer) TransferToHost(addr string) error {
	if strings.TrimSpace(addr) == "" {
		return errors.New("empty address")
	}
	if p.Protocol().Lower(version.Minecraft_1_20_5) {
		return fmt.Errorf("%w: but player is on %s", ErrTransferUnsupportedClientProtocol, p.Protocol())
	}

	host, port, err := net.SplitHostPort(addr)
	var portInt int
	if err != nil {
		host = addr
		portInt = 25565
	} else {
		portInt, err = strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("invalid port %s: %w", port, err)
		}
	}

	targetAddr := netutil.NewAddr(fmt.Sprintf("%s:%d", host, portInt), "tcp")
	f := future.NewChan[error]()
	event.FireParallel(p.eventMgr, newPreTransferEvent(p, targetAddr), func(e *PreTransferEvent) {
		defer f.Complete(nil)
		if e.Allowed() {
			resultedAddr := e.Addr()
			if resultedAddr != nil {
				resultedAddr = targetAddr
			}
			host, port := netutil.HostPort(resultedAddr)
			err = p.WritePacket(&packet.Transfer{
				Host: host,
				Port: int(port),
			})
			if err != nil {
				f.Complete(err)
				return
			}
			p.log.Info("transferring player to host", "host", resultedAddr)
		}
	})
	return f.Get()
}

func (p *connectedPlayer) BackendState() *states.State {
	backend, ok := p.ensureBackendConnection()
	if !ok {
		return nil
	}
	return &backend.State().State
}

func (p *connectedPlayer) BundleHandler() *resourcepack.BundleDelimiterHandler {
	return p.bundleHandler
}

func (p *connectedPlayer) BackendInFlight() proto.PacketWriter {
	if connInFlight := p.connectionInFlight(); connInFlight != nil {
		if mcConn := connInFlight.conn(); mcConn != nil {
			return mcConn
		}
	}
	return nil
}

// Discards any messages still being processed by the chat queue, and creates a fresh state for future packets.
// This should be used on server switches, or whenever the client resets its own 'last seen' state.
func (p *connectedPlayer) discardChatQueue() {
	// No need for atomic swap, should only be called from read loop
	oldChatQueue := p.chatQueue
	p.chatQueue = newChatQueue(p)
	oldChatQueue.close()
}

func (p *connectedPlayer) HandshakeIntent() packet.HandshakeIntent {
	return p.handshakeIntent
}
