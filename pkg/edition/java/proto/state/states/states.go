package states

// State represents the state of the protocol in which a connection can be present.
// It is a Java edition client state.
type State int

const (
	// HandshakeState is the initial connection state. This status can be caused by a HandshakeIntent's STATUS,
	// LOGIN or TRANSFER intent. If the intent is LOGIN or TRANSFER, the next state will be LOGIN,
	// otherwise, it will go to the STATUS state.
	HandshakeState State = 0

	// StatusState is the ping state of a connection. Connections with the HandshakeIntent's STATUS intent will pass through this state
	// and be disconnected after it requests the ping from the server and the server responds with the respective ping.
	StatusState State = 1

	// LoginState is the authentication state of a connection. At this moment the player is authenticating with the authentication servers.
	LoginState State = 2

	// PlayState is the game state of a connection. In this state is where the whole game runs, the server is able to change
	// the player's state to CONFIGURATION as needed in versions 1.20.2 and higher.
	PlayState State = 3

	// ConfigState is the configuration state of a connection. At this point the player allows the server to send information
	// such as resource packs and plugin messages, at the same time the player will send his client brand and the respective plugin messages
	// if it is a modded client. This state is available since Minecraft 1.20.2.
	ConfigState State = 4
)

// String implements fmt.Stringer.
func (s State) String() string {
	switch s {
	case HandshakeState:
		return "Handshake"
	case StatusState:
		return "Status"
	case ConfigState:
		return "Config"
	case LoginState:
		return "Login"
	case PlayState:
		return "Play"
	}
	return "UnknownState"
}
