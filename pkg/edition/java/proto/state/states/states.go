package states

// State is a Java edition client state.
type State int

// The states the Java edition client connection can be in.
const (
	HandshakeState State = 0
	StatusState    State = 1
	ConfigState    State = 4 // Minecraft 1.20.2+: After StatusState, before LoginState
	LoginState     State = 2
	PlayState      State = 3
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
