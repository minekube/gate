package proxy

import (
	"net"

	"go.minekube.com/gate/pkg/edition/java/proxy/internal/resourcepack"
	"go.minekube.com/gate/pkg/util/uuid"

	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/key"

	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/edition/java/proxy/player"
	"go.minekube.com/gate/pkg/util/permission"
)

// PingEvent is fired when a request for server information is sent by a remote client,
// or when the server sends the MOTD and favicon to the client after a successful login.
// The proxy will wait on this event to finish firing before delivering the results to
// the remote client, but you are urged to handle this event as quickly as possible when
// handling this event due to the amount of ping packets a client can send.
type PingEvent struct {
	inbound Inbound
	ping    *ping.ServerPing
}

// Connection returns the inbound connection.
func (p *PingEvent) Connection() Inbound {
	return p.inbound
}

// Ping returns the used ping. (pre-initialized by the proxy)
func (p *PingEvent) Ping() *ping.ServerPing {
	return p.ping
}

// SetPing sets the ping response to use.
func (p *PingEvent) SetPing(ping *ping.ServerPing) {
	p.ping = ping
}

//
//
//
//
//

// ConnectionEventConn tracks whether Close was called on the connection.
type ConnectionEventConn interface {
	net.Conn
	Closed() bool // Whether Close was called.
}

// ConnectionEvent is fired when a client connects with the proxy.
// It can be used for low-level connection handling, modifying or closing the connection.
// This event is fired before ConnectionHandshakeEvent
type ConnectionEvent struct {
	conn     net.Conn
	original ConnectionEventConn
}

// Connection returns the connection.
func (e *ConnectionEvent) Connection() net.Conn {
	return e.conn
}

// OriginalConnection returns the original connection.
func (e *ConnectionEvent) OriginalConnection() ConnectionEventConn {
	return e.original
}

// SetConnection sets new connection to use for this connection.
func (e *ConnectionEvent) SetConnection(conn net.Conn) {
	e.conn = conn
}

//
//
//
//
//

// ConnectionHandshakeEvent is fired when a handshake
// is established between a client and the proxy.
type ConnectionHandshakeEvent struct {
	inbound Inbound
	intent  packet.HandshakeIntent
}

// Connection returns the inbound connection.
func (e *ConnectionHandshakeEvent) Connection() Inbound {
	return e.inbound
}

// Intent returns the handshake intent.
func (e *ConnectionHandshakeEvent) Intent() packet.HandshakeIntent {
	return e.intent
}

//
//
//
//
//

// GameProfileRequestEvent is fired after the PreLoginEvent in
// order to set up the game profile for the user.
// This can be used to configure a custom profile for a user, i.e. skin replacement.
type GameProfileRequestEvent struct {
	inbound    Inbound
	original   profile.GameProfile
	onlineMode bool

	use profile.GameProfile
}

// NewGameProfileRequestEvent creates a new GameProfileRequestEvent.
func NewGameProfileRequestEvent(
	inbound Inbound,
	original profile.GameProfile,
	onlineMode bool,
) *GameProfileRequestEvent {
	return &GameProfileRequestEvent{
		inbound:    inbound,
		original:   original,
		onlineMode: onlineMode,
	}
}

// Conn returns the inbound connection that is connecting to the proxy.
func (e *GameProfileRequestEvent) Conn() Inbound {
	return e.inbound
}

// Original returns the by the proxy created offline or online (Mojang authenticated) game profile.
func (e *GameProfileRequestEvent) Original() profile.GameProfile {
	return e.original
}

// OnlineMode specifies whether the user connected in online/offline mode.
func (e *GameProfileRequestEvent) OnlineMode() bool {
	return e.onlineMode
}

// SetGameProfile sets the profile to use for this connection.
func (e *GameProfileRequestEvent) SetGameProfile(p profile.GameProfile) {
	e.use = p
}

// GameProfile returns the game profile that will be used to initialize the connection with.
// Should no profile be set, the original profile (given by the proxy) will be used.
func (e *GameProfileRequestEvent) GameProfile() profile.GameProfile {
	if len(e.use.Name) == 0 {
		return e.original
	}
	return e.use
}

//
//
//
//
//
//

// PlayerModInfoEvent is fired when a Forge client sends its
// mods to the proxy while connecting to a server.
type PlayerModInfoEvent struct {
	player  Player
	modInfo modinfo.ModInfo
}

// Player returns the player who sent the mod info.
func (e *PlayerModInfoEvent) Player() Player {
	return e.player
}

// ModInfo is the mod info received by the player.
func (e *PlayerModInfoEvent) ModInfo() modinfo.ModInfo {
	return e.modInfo
}

//
//
//
//
//
//
//
//

// PermissionsSetupEvent is fired once a permission.Subject's
// permissions are being initialized.
type PermissionsSetupEvent struct {
	subject     permission.Subject
	defaultFunc permission.Func

	fn permission.Func
}

// Subject returns the subject the permissions are setup for.
func (p *PermissionsSetupEvent) Subject() permission.Subject {
	return p.subject
}

// Func returns the permission.Func used for the subject.
func (p *PermissionsSetupEvent) Func() permission.Func {
	if p.fn == nil {
		return p.defaultFunc
	}
	return p.fn
}

// SetFunc sets the permission.Func usec for the subject.
// If fn is nil, the default Func fill be used.
func (p *PermissionsSetupEvent) SetFunc(fn permission.Func) {
	if fn == nil {
		return
	}
	p.fn = fn
}

//
//
//
//
//
//
//

// PreLoginEvent is fired when a player has initiated a connection with the proxy
// but before the proxy authenticates the player with Mojang or before the player's proxy connection
// is fully established (for offline mode).
type PreLoginEvent struct {
	connection Inbound
	username   string
	id         uuid.UUID // player's uuid, nil-able

	result PreLoginResult
	reason component.Component
}

func newPreLoginEvent(conn Inbound, username string, id uuid.UUID) *PreLoginEvent {
	return &PreLoginEvent{
		connection: conn,
		username:   username,
		id:         id,
		result:     AllowedPreLogin,
	}
}

// PreLoginResult is the result of a PreLoginEvent.
type PreLoginResult uint8

// PreLoginResult values.
const (
	AllowedPreLogin PreLoginResult = iota
	DeniedPreLogin
	ForceOnlineModePreLogin
	ForceOfflineModePreLogin
)

// Username returns the username of the player.
func (e *PreLoginEvent) Username() string {
	return e.username
}

// ID returns the UUID of the connecting player. May be uuid.Nil!
// This value is nil on 1.19.2 and lower,
// up to 1.20.1 it is optional and from 1.20.2 it will always be available.
func (e *PreLoginEvent) ID() (uuid.UUID, bool) {
	return e.id, e.id != uuid.Nil
}

// Conn returns the inbound connection that is connecting to the proxy.
func (e *PreLoginEvent) Conn() Inbound {
	return e.connection
}

// Result returns the current result of the PreLoginEvent.
func (e *PreLoginEvent) Result() PreLoginResult {
	return e.result
}

// Reason returns the `deny reason` to disconnect the connection.
// May be nil!
func (e *PreLoginEvent) Reason() component.Component {
	return e.reason
}

func (e *PreLoginEvent) Deny(reason component.Component) {
	e.result = DeniedPreLogin
	e.reason = reason
}

func (e *PreLoginEvent) Allow() {
	e.result = AllowedPreLogin
	e.reason = nil
}

func (e *PreLoginEvent) ForceOnlineMode() {
	e.result = ForceOnlineModePreLogin
	e.reason = nil
}

func (e *PreLoginEvent) ForceOfflineMode() {
	e.result = ForceOfflineModePreLogin
	e.reason = nil
}

//
//
//
//
//
//
//
//

type LoginEvent struct {
	player Player

	denied bool
	reason component.Component
}

func (e *LoginEvent) Player() Player {
	return e.player
}

func (e *LoginEvent) Deny(reason component.Component) {
	e.denied = true
	e.reason = reason
}

func (e *LoginEvent) Allow() {
	e.denied = false
	e.reason = nil
}

func (e *LoginEvent) Allowed() bool {
	return !e.denied
}

// Is nil if Allowed() returns true
func (e *LoginEvent) Reason() component.Component {
	return e.reason
}

//
//
//
//
//
//
//

type DisconnectEvent struct {
	player      Player
	loginStatus LoginStatus
}

type LoginStatus uint8

const (
	SuccessfulLoginStatus LoginStatus = iota
	ConflictingLoginStatus
	CanceledByUserLoginStatus
	CanceledByProxyLoginStatus
	CanceledByUserBeforeCompleteLoginStatus
)

func (e *DisconnectEvent) Player() Player {
	return e.player
}

func (e *DisconnectEvent) LoginStatus() LoginStatus {
	return e.loginStatus
}

//
//
//
//
//
//
//
//

type PostLoginEvent struct {
	player Player
}

func (e *PostLoginEvent) Player() Player {
	return e.player
}

//
//
//
//
//
//

// PlayerChooseInitialServerEvent is fired when a player has finished the login process,
// and we need to choose the first server to connect to.
// The proxy will wait on this event to finish firing before initiating the connection
// but you should try to limit the work done in this event.
// Failures will be handled by KickedFromServerEvent as normal.
type PlayerChooseInitialServerEvent struct {
	player        Player
	initialServer RegisteredServer // May be nil if no server is configured.
}

// Player returns the player to find the initial server for.
func (e *PlayerChooseInitialServerEvent) Player() Player {
	return e.player
}

// InitialServer returns the initial server or nil if no server is configured.
func (e *PlayerChooseInitialServerEvent) InitialServer() RegisteredServer {
	return e.initialServer
}

// SetInitialServer sets the initial server for the player.
func (e *PlayerChooseInitialServerEvent) SetInitialServer(server RegisteredServer) {
	e.initialServer = server
}

//
//
//
//
//
//

// ServerPreConnectEvent is fired before the player connects to a server.
type ServerPreConnectEvent struct {
	player         Player
	original       RegisteredServer
	server         RegisteredServer
	previousServer RegisteredServer // nil-able
}

func newServerPreConnectEvent(player Player, server RegisteredServer, previousServer RegisteredServer) *ServerPreConnectEvent {
	return &ServerPreConnectEvent{
		player:         player,
		original:       server,
		server:         server,
		previousServer: previousServer,
	}
}

// Player returns the player that tries to connect to another server.
func (e *ServerPreConnectEvent) Player() Player {
	return e.player
}

// OriginalServer returns the server that the player originally tried to connect to.
// To get the server the player will connect to, see the Server() of this event.
// To get the server the player is currently on when this event is fired, use Player.getCurrentServer().
func (e *ServerPreConnectEvent) OriginalServer() RegisteredServer {
	return e.original
}

// Allow the player to connect to the specified server.
func (e *ServerPreConnectEvent) Allow(server RegisteredServer) {
	e.server = server
}

// Deny will cancel the player to connect to another server.
func (e *ServerPreConnectEvent) Deny() {
	e.server = nil
}

// Allowed returns true whether the connection is allowed.
func (e *ServerPreConnectEvent) Allowed() bool {
	return e.server != nil
}

// Server returns the server the player will connect to OR
// nil if Allowed() returns false.
func (e *ServerPreConnectEvent) Server() RegisteredServer {
	return e.server
}

// PreviousServer returns the server the player was previously connected to.
// May return nil if there was none!
func (e *ServerPreConnectEvent) PreviousServer() RegisteredServer {
	return e.previousServer
}

//
//
//
//
//
//

// PreTransferEvent is fired before a player is transferred to another host,
// either by the backend server or by a plugin using the Player.TransferTo method.
type PreTransferEvent struct {
	player       Player
	originalAddr net.Addr
	targetAddr   net.Addr
	denied       bool
}

func newPreTransferEvent(player Player, addr net.Addr) *PreTransferEvent {
	return &PreTransferEvent{
		player:       player,
		originalAddr: addr,
		targetAddr:   addr,
	}
}

// TransferTo changes the target address the player will be transferred to.
func (e *PreTransferEvent) TransferTo(addr net.Addr) {
	e.targetAddr = addr
	e.denied = false
}

// Addr returns the target address the player will be transferred to.
func (e *PreTransferEvent) Addr() net.Addr {
	return e.targetAddr
}

// Allowed returns true if the transfer is allowed.
func (e *PreTransferEvent) Allowed() bool {
	return !e.denied
}

// Player returns the player that is about to be transferred.
func (e *PreTransferEvent) Player() Player {
	return e.player
}

//
//
//
//
//
//

// Fired when a player is kicked from a server. You may either allow the proxy to kick the player
// (with an optional reason override) or redirect the player to a separate server. By default,
// the proxy will notify the user (if they are already connected to a server) or disconnect them
// (if they are not on a server and no other servers are available).
type KickedFromServerEvent struct {
	player              Player
	server              RegisteredServer
	originalReason      component.Component // May be nil!
	duringServerConnect bool

	result ServerKickResult
}

// ServerKickResult is the result of a KickedFromServerEvent and is implemented by
//
// # DisconnectPlayerKickResult
//
// # RedirectPlayerKickResult
//
// NotifyKickResult
type ServerKickResult interface {
	isServerKickResult() // assert implemented internally
}

var (
	_ ServerKickResult = (*DisconnectPlayerKickResult)(nil)
	_ ServerKickResult = (*RedirectPlayerKickResult)(nil)
	_ ServerKickResult = (*NotifyKickResult)(nil)
)

func newKickedFromServerEvent(
	player Player, server RegisteredServer,
	reason component.Component, duringServerConnect bool,
	initialResult ServerKickResult,
) *KickedFromServerEvent {
	return &KickedFromServerEvent{
		player:              player,
		server:              server,
		originalReason:      reason,
		duringServerConnect: duringServerConnect,
		result:              initialResult,
	}
}

// Player returns the player that got kicked.
func (e *KickedFromServerEvent) Player() Player {
	return e.player
}

// Server returns the server the player got kicked from.
func (e *KickedFromServerEvent) Server() RegisteredServer {
	return e.server
}

// OriginalReason returns the reason the server kicked the player from the server.
// May return nil!
func (e *KickedFromServerEvent) OriginalReason() component.Component {
	return e.originalReason
}

// KickedDuringServerConnect returns true if the player got kicked while connecting to another server.
func (e *KickedFromServerEvent) KickedDuringServerConnect() bool {
	return e.duringServerConnect
}

// Result returns current kick result.
// The proxy sets a default non-nil result but an event handler
// may has set it nil when handling the event.
func (e *KickedFromServerEvent) Result() ServerKickResult {
	return e.result
}

// SetResult sets the kick result.
func (e *KickedFromServerEvent) SetResult(result ServerKickResult) {
	e.result = result
}

// DisconnectPlayerKickResult is a ServerKickResult and
// tells the proxy to disconnect the player with the specified reason.
type DisconnectPlayerKickResult struct {
	Reason component.Component
}

func (*DisconnectPlayerKickResult) isServerKickResult() {}

// RedirectPlayerKickResult is a ServerKickResult and
// tells the proxy to redirect the player to another server.
type RedirectPlayerKickResult struct {
	Server  RegisteredServer    // The new server to redirect the kicked player to.
	Message component.Component // Optional message sent to the player after redirecting.
}

func (*RedirectPlayerKickResult) isServerKickResult() {}

// NotifyKickResult is ServerKickResult and
// notifies the player with the specified message but does nothing else.
// This is only a valid result to use if the player was trying to connect
// to a different server, otherwise it is treated like a DisconnectPlayerKickResult result.
type NotifyKickResult struct {
	Message component.Component
}

func (*NotifyKickResult) isServerKickResult() {}

//
//
//
//
//
//

// ServerConnectedEvent is fired before the player completely transitions
// to the target server and the connection to the previous server has been
// de-established.
//
// Use Server to get the target server since Player.CurrentServer is yet nil or
// listen for ServerPostConnectEvent instead.
type ServerConnectedEvent struct {
	player         Player
	server         RegisteredServer
	previousServer RegisteredServer // nil-able
	entityID       int
}

// Player returns the associated player.
func (s *ServerConnectedEvent) Player() Player {
	return s.player
}

// Server returns the server the player connected to.
func (s *ServerConnectedEvent) Server() RegisteredServer {
	return s.server
}

// PreviousServer returns the server the player was previously connected to.
// May return nil if there was none!
func (s *ServerConnectedEvent) PreviousServer() RegisteredServer {
	return s.previousServer
}

// EntityID returns the current entity ID of the player.
// This is the entity ID the player has on the connected server and changes
// every time the player connects to a new server.
func (s *ServerConnectedEvent) EntityID() int {
	return s.entityID
}

//
//
//
//
//

// ServerPostConnectEvent is fired after the player has connected to a server.
// The server the player is now connected to is available in Player().CurrentServer().
type ServerPostConnectEvent struct {
	player         Player
	previousServer RegisteredServer // nil-able
}

func newServerPostConnectEvent(player Player, previousServer RegisteredServer) *ServerPostConnectEvent {
	return &ServerPostConnectEvent{player: player, previousServer: previousServer}
}

// Player returns the associated player.
func (s *ServerPostConnectEvent) Player() Player {
	return s.player
}

// PreviousServer returns the server the player was previously connected to.
// May return nil if there was none!
func (s *ServerPostConnectEvent) PreviousServer() RegisteredServer {
	return s.previousServer
}

//
//
//
//
//

// PluginMessageEvent is fired when a plugin message is sent to the proxy,
// either from a player or a server backend server.
type PluginMessageEvent struct {
	source     message.ChannelMessageSource
	target     message.ChannelMessageSink
	identifier message.ChannelIdentifier
	data       []byte

	forward bool
}

func (p *PluginMessageEvent) Source() message.ChannelMessageSource {
	return p.source
}
func (p *PluginMessageEvent) Target() message.ChannelMessageSink {
	return p.target
}
func (p *PluginMessageEvent) Identifier() message.ChannelIdentifier {
	return p.identifier
}
func (p *PluginMessageEvent) Data() []byte {
	return p.data
}
func (p *PluginMessageEvent) SetForward(forward bool) {
	p.forward = forward
}
func (p *PluginMessageEvent) Allowed() bool {
	return p.forward
}

//
//
//
//
//

type PlayerSettingsChangedEvent struct {
	player   Player
	settings player.Settings
}

// Player returns the player whose settings where updates/initialized.
func (s *PlayerSettingsChangedEvent) Player() Player {
	return s.player
}

// Settings returns player's new settings.
func (s *PlayerSettingsChangedEvent) Settings() player.Settings {
	return s.settings
}

//
//
//
//

// PlayerChatEvent is fired when a player sends a chat message.
// Note that messages with a leading "/" do not trigger this event, but instead CommandExecuteEvent.
type PlayerChatEvent struct {
	player   Player
	original string
	modified string

	denied bool
}

// Player returns the player that sent the message.
func (c *PlayerChatEvent) Player() Player {
	return c.player
}

// Message returns the message that will be sent by the player.
func (c *PlayerChatEvent) Message() string {
	if c.modified == "" {
		return c.original
	}
	return c.modified
}

// SetMessage modifies the message of the player.
func (c *PlayerChatEvent) SetMessage(msg string) {
	if msg == c.original {
		return // not modified
	}
	c.modified = msg
}

// Original returns the original message the player sent.
func (c *PlayerChatEvent) Original() string {
	return c.original
}

// SetAllowed sets whether the chat message is allowed.
// Deprecated: for 1.19.1 and newer, set this as denied will kick users.
func (c *PlayerChatEvent) SetAllowed(allowed bool) {
	c.denied = !allowed
}

// Allowed returns true when the chat message is allowed.
func (c *PlayerChatEvent) Allowed() bool {
	return !c.denied
}

//
//
//
//
//

// CommandExecuteEvent is fired when someone wants to execute a command.
type CommandExecuteEvent struct {
	source          command.Source
	commandline     string
	originalCommand string

	forward bool // forward command to server
	denied  bool
}

// Source returns the command source that wants to run the command.
func (c *CommandExecuteEvent) Source() command.Source {
	return c.source
}

// Command returns the whole commandline without the leading "/".
func (c *CommandExecuteEvent) Command() string {
	return c.commandline
}

// OriginalCommand returns the original command if SetCommand has changed it.
func (c *CommandExecuteEvent) OriginalCommand() string {
	return c.originalCommand
}

// SetCommand changes the command being executed without the leading "/".
func (c *CommandExecuteEvent) SetCommand(commandline string) {
	c.commandline = commandline
}

// SetAllowed sets whether the command is allowed to be executed.
func (c *CommandExecuteEvent) SetAllowed(allowed bool) {
	c.denied = !allowed
}

// Allowed returns true when the command is allowed to be executed.
func (c *CommandExecuteEvent) Allowed() bool {
	return !c.denied
}

// SetForward sets whether the command should be forwarded to the server.
func (c *CommandExecuteEvent) SetForward(forward bool) {
	c.forward = forward
}

// Forward returns true when the command should be forwarded to the server.
func (c *CommandExecuteEvent) Forward() bool {
	return c.forward
}

//
//
//
//

// TabCompleteEvent is fired after a tab complete response is sent by the remote server,
// for clients on1.12.2 and below. You have the opportunity to modify the response sent
// to the remote player.
type TabCompleteEvent struct {
	player         Player
	partialMessage string
	suggestions    []string
}

// Player returns the player requesting the tab completion.
func (t *TabCompleteEvent) Player() Player {
	return t.player
}

// Suggestions returns all the suggestions provided to the user, as a mutable list.
func (t *TabCompleteEvent) Suggestions() []string {
	return t.suggestions
}

// SetSuggestions sets the suggestions provided to the user.
func (t *TabCompleteEvent) SetSuggestions(s []string) {
	t.suggestions = s
}

// PartialMessage returns the message being partially completed.
func (t *TabCompleteEvent) PartialMessage() string {
	return t.partialMessage
}

//
//
//
//

// PlayerAvailableCommandsEvent allows plugins to modify the packet
// indicating commands available on the server to a Minecraft 1.13+ client.
type PlayerAvailableCommandsEvent struct {
	player   Player
	rootNode *brigodier.RootCommandNode
}

// Player returns the player that is about to see the available commands.
func (p *PlayerAvailableCommandsEvent) Player() Player {
	return p.player
}

// RootNode returns the available commands to the Player.
func (p *PlayerAvailableCommandsEvent) RootNode() *brigodier.RootCommandNode {
	return p.rootNode
}

//
//
//
//

// ResourcePackResponseStatus represents the possible statuses for the resource pack.
type ResourcePackResponseStatus = resourcepack.ResponseStatus

// Possible statuses for a resource pack.

const (
	// SuccessfulResourcePackResponseStatus indicates the resource pack was applied successfully.
	SuccessfulResourcePackResponseStatus ResourcePackResponseStatus = resourcepack.SuccessfulResponseStatus
	// DeclinedResourcePackResponseStatus indicates the player declined to download the resource pack.
	DeclinedResourcePackResponseStatus ResourcePackResponseStatus = resourcepack.DeclinedResponseStatus
	// FailedDownloadResourcePackResponseStatus indicates the player could not download the resource pack.
	FailedDownloadResourcePackResponseStatus ResourcePackResponseStatus = resourcepack.FailedDownloadResponseStatus
	// AcceptedResourcePackResponseStatus indicates the player has accepted the resource pack and is now downloading it.
	AcceptedResourcePackResponseStatus ResourcePackResponseStatus = resourcepack.AcceptedResponseStatus
	// DownloadedResourcePackResponseStatus indicates the player has downloaded the resource pack.
	DownloadedResourcePackResponseStatus ResourcePackResponseStatus = resourcepack.DownloadedResponseStatus
	// InvalidURLResourcePackResponseStatus indicates the URL of the resource pack failed to load.
	InvalidURLResourcePackResponseStatus ResourcePackResponseStatus = resourcepack.InvalidURLResponseStatus
	// FailedToReloadResourcePackResponseStatus indicates the player failed to reload the resource pack.
	FailedToReloadResourcePackResponseStatus ResourcePackResponseStatus = resourcepack.FailedToReloadResponseStatus
	// DiscardedResourcePackResponseStatus indicates the resource pack was discarded.
	DiscardedResourcePackResponseStatus ResourcePackResponseStatus = resourcepack.DiscardedResponseStatus
)

// PlayerResourcePackStatusEvent is fired when the status of a resource pack sent to the player by the server is
// changed. Depending on the result of this event (which the proxy will wait until completely fired),
// the player may be kicked from the server.
type PlayerResourcePackStatusEvent = resourcepack.PlayerResourcePackStatusEvent

//
//
//
//

// ServerResourcePackSendEvent is fired when the downstream server tries to send a player a ResourcePack packet.
// The proxy will wait on this event to finish before forwarding the resource pack to the user.
// If this event is denied, it will retroactively send a DENIED status to the downstream server in response.
// If the downstream server has it set to "forced" it will forcefully disconnect the user.
type ServerResourcePackSendEvent struct {
	denied               bool
	receivedResourcePack ResourcePackInfo
	providedResourcePack ResourcePackInfo
	serverConn           *serverConnection
}

// newServerResourcePackSendEvent creates a new ServerResourcePackSendEvent.
func newServerResourcePackSendEvent(
	packInfo ResourcePackInfo,
	serverConn *serverConnection,
) *ServerResourcePackSendEvent {
	return &ServerResourcePackSendEvent{
		receivedResourcePack: packInfo,
		providedResourcePack: packInfo,
		serverConn:           serverConn,
	}
}

// Allowed indicated whether sending the resource pack to the client is allowed.
func (e *ServerResourcePackSendEvent) Allowed() bool {
	return !e.denied
}

// SetAllowed allows or denies sending the resource pack to the client.
func (e *ServerResourcePackSendEvent) SetAllowed(allowed bool) {
	e.denied = !allowed
}

// ServerConnection returns the associated server connection.
func (e *ServerResourcePackSendEvent) ServerConnection() ServerConnection {
	return e.serverConn
}

// ReceivedResourcePack returns the resource pack send by the server.
func (e *ServerResourcePackSendEvent) ReceivedResourcePack() ResourcePackInfo {
	return e.receivedResourcePack
}

// ProvidedResourcePack returns the resource pack provided to the client if allowed.
func (e *ServerResourcePackSendEvent) ProvidedResourcePack() ResourcePackInfo {
	return e.providedResourcePack
}

// SetProvidedResourcePack sets the resource pack provided to the client if allowed.
func (e *ServerResourcePackSendEvent) SetProvidedResourcePack(pack ResourcePackInfo) {
	e.providedResourcePack = pack
}

//
//
//
//

// PlayerChannelRegisterEvent is fired when a client Player sends a plugin message through the
// register channel. The proxy will not wait on this event to finish firing.
type PlayerChannelRegisterEvent struct {
	channels []message.ChannelIdentifier
	player   Player
}

func (e *PlayerChannelRegisterEvent) Channels() []message.ChannelIdentifier {
	return e.channels
}

func (e *PlayerChannelRegisterEvent) Player() Player {
	return e.player
}

//
//
//
//

// PlayerChannelUnregisterEvent is fired when a client Player sends a plugin message through the
// unregister channel. The proxy will not wait on this event to finish firing.
type PlayerChannelUnregisterEvent struct {
	channels []message.ChannelIdentifier
	player   Player
}

func (e *PlayerChannelUnregisterEvent) Channels() []message.ChannelIdentifier {
	return e.channels
}

func (e *PlayerChannelUnregisterEvent) Player() Player {
	return e.player
}

//
//
//
//

// ServerLoginPluginMessageEvent is fired when a server sends a login plugin message to the proxy.
// Plugins have the opportunity to respond to the messages as needed. The proxy will wait on this
// event to finish. The server will be responsible for continuing the login process once the server
// is satisfied with any login plugin responses sent by proxy plugins (or messages indicating a lack of response).
type ServerLoginPluginMessageEvent struct {
	id         message.ChannelIdentifier
	contents   []byte
	sequenceID int
	serverConn *serverConnection

	result ServerLoginPluginMessageResult
}

// Contents returns the contents of the login plugin message sent by the server.
func (e *ServerLoginPluginMessageEvent) Contents() []byte {
	return e.contents
}

// SequenceID returns the sequence id of the login plugin message sent by the server.
func (e *ServerLoginPluginMessageEvent) SequenceID() int {
	return e.sequenceID
}

// ServerConnection returns the associated server connection.
func (e *ServerLoginPluginMessageEvent) ServerConnection() ServerConnection {
	return e.serverConn
}

func (e *ServerLoginPluginMessageEvent) Result() *ServerLoginPluginMessageResult {
	return &e.result
}

type ServerLoginPluginMessageResult struct {
	Response []byte
}

func (r *ServerLoginPluginMessageResult) Allowed() bool {
	return r.Response != nil
}

func (r *ServerLoginPluginMessageResult) Copy() []byte {
	res := make([]byte, len(r.Response))
	copy(res, r.Response)
	return res
}

func (r *ServerLoginPluginMessageResult) Reply(response []byte) *ServerLoginPluginMessageResult {
	return &ServerLoginPluginMessageResult{
		Response: response,
	}
}

//
//
//
//

// PlayerClientBrandEvent is fired when a Player sends the `minecraft:brand` plugin message.
// The proxy will not wait on event handlers to finish firing.
type PlayerClientBrandEvent struct {
	player Player
	brand  string
}

func (e *PlayerClientBrandEvent) Player() Player {
	return e.player
}
func (e *PlayerClientBrandEvent) Brand() string {
	return e.brand
}

//
//
//
//

// PreShutdownEvent is fired by the proxy after it has stopped accepting new connections,
// but before any players are disconnected. This is the last opportunity to interact with
// currently connected players, such as transferring them to another proxy or performing
// cleanup tasks.
//
// The proxy will wait for all event listeners to complete before disconnecting players.
type PreShutdownEvent struct {
	reason component.Component // may be nil
}

// Reason returns the shutdown reason used to disconnect players with.
// May be nil!
func (s *PreShutdownEvent) Reason() component.Component {
	return s.reason
}

// SetReason sets the shutdown reason used to disconnect players with.
func (s *PreShutdownEvent) SetReason(reason component.Component) {
	s.reason = reason
}

//
//
//
//

// ReadyEvent is fired once the proxy was successfully
// initialized and is ready to serve connections.
//
// May be triggered multiple times on config reloads.
type ReadyEvent struct {
	addr string // The address the proxy is listening on.
}

// Addr returns the address the proxy is listening on.
func (r *ReadyEvent) Addr() string { return r.addr }

// ShutdownEvent is fired by the proxy after the proxy
// has stopped accepting connections and PreShutdownEvent,
// but before the proxy process exits.
//
// Subscribe to this event to gracefully stop any subtasks,
// such as plugin dependencies.
type ShutdownEvent struct{}

//
//
//
//

// CookieReceiveEvent is fired when a cookie from a client is requested either by a proxy plugin or
// by a backend server. Gate will wait on this event to finish firing before discarding the
// cookie request (if handled) or forwarding it to the client.
type CookieReceiveEvent struct {
	player          Player
	key             key.Key
	originalKey     key.Key
	payload         []byte
	originalPayload []byte
	denied          bool
}

func newCookieReceiveEvent(player Player, key key.Key, payload []byte) *CookieReceiveEvent {
	return &CookieReceiveEvent{
		player:          player,
		key:             key,
		payload:         payload,
		originalKey:     key,
		originalPayload: payload,
		denied:          false,
	}
}

// Player returns the player from whom the cookie has been received.
func (c *CookieReceiveEvent) Player() Player { return c.player }

// Key returns the provider of the responded cookie.
// For example: minecraft:cookie
func (c *CookieReceiveEvent) Key() key.Key { return c.key }

func (c *CookieReceiveEvent) SetKey(key key.Key) { c.key = key }

// OriginalKey returns the original key of the cookie request.
func (c *CookieReceiveEvent) OriginalKey() key.Key { return c.originalKey }

// Payload returns the payload of the responded cookie.
func (c *CookieReceiveEvent) Payload() []byte { return c.payload }

// SetPayload sets the payload of the responded cookie.
func (c *CookieReceiveEvent) SetPayload(payload []byte) { c.payload = payload }

// OriginalPayload returns the original payload of the cookie request.
func (c *CookieReceiveEvent) OriginalPayload() []byte { return c.originalPayload }

// Allowed returns whether the cookie request is allowed to be forwarded to the client.
func (c *CookieReceiveEvent) Allowed() bool { return !c.denied }

// SetAllowed sets whether the cookie request is allowed to be forwarded to the client.
func (c *CookieReceiveEvent) SetAllowed(allowed bool) { c.denied = !allowed }

//
//
//
//

// CookieStoreEvent is fired when a cookie should be stored on a player's client. This process can be
// initiated either by a proxy plugin or by a backend server. Gate will wait on this event
// to finish firing before discarding the cookie (if handled) or forwarding it to the client so
// that it can store the cookie.
type CookieStoreEvent struct {
	player          Player
	key             key.Key
	payload         []byte
	originalKey     key.Key
	originalPayload []byte
	denied          bool
}

func newCookieStoreEvent(player Player, key key.Key, payload []byte) *CookieStoreEvent {
	return &CookieStoreEvent{
		player:          player,
		key:             key,
		payload:         payload,
		originalKey:     key,
		originalPayload: payload,
		denied:          false,
	}
}

// Player returns the player from whom the cookie has been received.
func (e *CookieStoreEvent) Player() Player { return e.player }

// Key returns the provider of the responded cookie.
// For example: minecraft:cookie
func (e *CookieStoreEvent) Key() key.Key { return e.key }

// SetKey sets the provider of the responded cookie.
func (e *CookieStoreEvent) SetKey(key key.Key) { e.key = key }

// Payload returns the payload of the responded cookie.
func (e *CookieStoreEvent) Payload() []byte { return e.payload }

// SetPayload sets the payload of the responded cookie.
func (e *CookieStoreEvent) SetPayload(payload []byte) { e.payload = payload }

// OriginalKey returns the original key of the cookie request.
func (e *CookieStoreEvent) OriginalKey() key.Key { return e.originalKey }

// OriginalPayload returns the original payload of the cookie request.
func (e *CookieStoreEvent) OriginalPayload() []byte { return e.originalPayload }

// Allowed returns whether the cookie request is allowed to be forwarded to the client.
func (e *CookieStoreEvent) Allowed() bool { return !e.denied }

// SetAllowed sets whether the cookie request is allowed to be forwarded to the client.
func (e *CookieStoreEvent) SetAllowed(allowed bool) { e.denied = !allowed }

//
//
//
//

// CookieRequestEvent is fired when a cookie from a client is requested either by a proxy plugin or
// by a backend server. Gate will wait on this event to finish firing before discarding the
// cookie request (if handled) or forwarding it to the client.
type CookieRequestEvent struct {
	player      Player
	key         key.Key
	originalKey key.Key
	denied      bool
}

func newCookieRequestEvent(player Player, key key.Key) *CookieRequestEvent {
	return &CookieRequestEvent{
		player:      player,
		key:         key,
		originalKey: key,
		denied:      false,
	}
}

// Player returns the player from whom the cookie has been requested.
func (e *CookieRequestEvent) Player() Player { return e.player }

// Key returns the provider of the requested cookie.
// For example: minecraft:cookie
func (e *CookieRequestEvent) Key() key.Key { return e.key }

// SetKey sets the provider of the requested cookie.
func (e *CookieRequestEvent) SetKey(key key.Key) { e.key = key }

// OriginalKey returns the original key of the cookie request.
func (e *CookieRequestEvent) OriginalKey() key.Key { return e.originalKey }

// Allowed returns whether the cookie request is allowed to be forwarded to the client.
func (e *CookieRequestEvent) Allowed() bool { return !e.denied }

// SetAllowed sets whether the cookie request is allowed to be forwarded to the client.
func (e *CookieRequestEvent) SetAllowed(allowed bool) { e.denied = !allowed }

//
//
//
//
//

// ServerRegisteredEvent is fired when a backend server is registered with the proxy.
// This allows plugins to react to dynamically added servers and perform necessary setup.
type ServerRegisteredEvent struct {
	server RegisteredServer
}

// Server returns the server that was registered.
func (e *ServerRegisteredEvent) Server() RegisteredServer {
	return e.server
}

//
//
//
//
//

// ServerUnregisteredEvent is fired when a backend server is unregistered from the proxy.
// This allows plugins to react to removed servers and perform necessary cleanup.
type ServerUnregisteredEvent struct {
	server ServerInfo
}

// ServerInfo returns the server info of the server that was unregistered.
func (e *ServerUnregisteredEvent) ServerInfo() ServerInfo {
	return e.server
}
