package proxy

import (
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/proxy/message"
	"go.minekube.com/gate/pkg/proxy/permission"
	"go.minekube.com/gate/pkg/proxy/ping"
	"go.minekube.com/gate/pkg/proxy/player"
	"go.minekube.com/gate/pkg/util/modinfo"
	"go.minekube.com/gate/pkg/util/profile"
)

// PingEvent is fired when a server list ping
// request is sent by a remote client.
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

// ConnectionHandshakeEvent is fired when a handshake
// is established between a client and the proxy.
type ConnectionHandshakeEvent struct {
	inbound Inbound
}

// Connection returns the inbound connection.
func (e *ConnectionHandshakeEvent) Connection() Inbound {
	return e.inbound
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

// minecraftConn returns the inbound connection that is connecting to the proxy.
func (e *GameProfileRequestEvent) Conn() Inbound {
	return e.inbound
}

// OriginalServer returns the by the proxy created offline or online (Mojang authenticated) game profile.
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

// SetFunc sets the permission.Func use for the subject.
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

type PreLoginEvent struct {
	connection Inbound
	username   string

	result PreLoginResult
	reason component.Component
}

func newPreLoginEvent(conn Inbound, username string) *PreLoginEvent {
	return &PreLoginEvent{
		connection: conn,
		username:   username,
		result:     AllowedPreLogin,
	}
}

type PreLoginResult uint8

const (
	AllowedPreLogin PreLoginResult = iota
	DeniedPreLogin
	ForceOnlineModePreLogin
	ForceOfflineModePreLogin
)

func (e *PreLoginEvent) Username() string {
	return e.username
}

func (e *PreLoginEvent) Conn() Inbound {
	return e.connection
}

func (e *PreLoginEvent) Result() PreLoginResult {
	return e.result
}

// Reason returns the deny reason to disconnect the connection.
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

// PlayerChooseInitialServerEvent is fired when a player has finished
// connecting to the proxy and we need to choose the first server to connect to.
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
	player   Player
	original RegisteredServer

	server RegisteredServer
}

func newServerPreConnectEvent(player Player, server RegisteredServer) *ServerPreConnectEvent {
	return &ServerPreConnectEvent{
		player:   player,
		original: server,
		server:   server,
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
// DisconnectPlayerKickResult
//
// RedirectPlayerKickResult
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

// KickedDuringServerConnect returns current kick result.
// The proxy sets a default non-nil result but an event handler
// may has set it nil when handling the event.
func (e *KickedFromServerEvent) Result() ServerKickResult {
	return e.result
}

// KickedDuringServerConnect sets the kick result.
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
// No messages will be sent from the proxy when this result is used.
type RedirectPlayerKickResult struct {
	Server  RegisteredServer    // The new server to redirect the kicked player to.
	Message component.Component // Optional message to send to the kicked player.
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

// ServerConnectedEvent is fired once the player has successfully
// connected to the target server and the connection to the previous
// server has been de-established (if any).
type ServerConnectedEvent struct {
	player         Player
	server         RegisteredServer
	previousServer RegisteredServer // nil-able
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

//
//
//
//
//

// Fired after the player has connected to a server.
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

type PluginMessageForwardResult struct {
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

// Player returns the player who's settings where updates/initialized.
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
	player  Player
	message string

	denied bool
}

// Player returns the player that sent the message.
func (c *PlayerChatEvent) Player() Player {
	return c.player
}

// Message returns the message the player sent.
func (c *PlayerChatEvent) Message() string {
	return c.message
}

// SetAllowed sets whether the chat message is allowed.
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
	source      CommandSource
	commandline string

	denied bool
}

// Source returns the command source that wants to run the command.
func (c *CommandExecuteEvent) Source() CommandSource {
	return c.source
}

// Command returns the whole commandline without the leading "/".
func (c *CommandExecuteEvent) Command() string {
	return c.commandline
}

// SetAllowed sets whether the command is allowed to be executed.
func (c *CommandExecuteEvent) SetAllowed(allowed bool) {
	c.denied = !allowed
}

// Allowed returns true when the command is allowed to be executed.
func (c *CommandExecuteEvent) Allowed() bool {
	return !c.denied
}

//
//
//
//

// PreShutdownEvent is fired before the proxy begins to shut down by
// stopping to accept new connections and disconnect all players.
type PreShutdownEvent struct {
	reason component.Component // may be nil
}

// Reason returns the shutdown reason used to disconnect players with.
// May be nil!
func (s *PreShutdownEvent) Reason() component.Component {
	return s.reason
}

// Reason returns the shutdown reason used to disconnect players with.
func (s *PreShutdownEvent) SetReason(reason component.Component) {
	s.reason = reason
}

//
//
//
//

// ShutdownEvent is fired by the proxy after the proxy
// has stopped accepting connections and PreShutdownEvent,
// but before the proxy process exits.
//
// Subscribe to this event to gracefully stop any subtasks,
// such as plugin dependencies.
type ShutdownEvent struct{}
