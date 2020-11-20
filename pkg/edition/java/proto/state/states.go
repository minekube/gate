package state

import (
	p "go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
)

// State is a Java edition client state.
type State int

// The states the Java edition client connection can be in.
const (
	HandshakeState State = iota
	StatusState
	LoginState
	PlayState
)

// The registries storing the packets for a connection state.
var (
	Handshake = NewRegistry(HandshakeState)
	Status    = NewRegistry(StatusState)
	Login     = NewRegistry(LoginState)
	Play      = NewRegistry(PlayState)
)

func init() {
	Handshake.ServerBound.Register(&p.Handshake{},
		m(0x00, version.Minecraft_1_7_2))

	Status.ServerBound.Register(&p.StatusRequest{},
		m(0x00, version.Minecraft_1_7_2))
	Status.ServerBound.Register(&p.StatusPing{},
		m(0x01, version.Minecraft_1_7_2))

	Status.ClientBound.Register(&p.StatusResponse{},
		m(0x00, version.Minecraft_1_7_2))
	Status.ClientBound.Register(&p.StatusPing{},
		m(0x01, version.Minecraft_1_7_2))

	Login.ServerBound.Register(&p.ServerLogin{},
		m(0x00, version.Minecraft_1_7_2))
	Login.ServerBound.Register(&p.EncryptionResponse{},
		m(0x01, version.Minecraft_1_7_2))
	Login.ServerBound.Register(&p.LoginPluginResponse{},
		m(0x02, version.Minecraft_1_7_2))

	Login.ClientBound.Register(&p.Disconnect{},
		m(0x00, version.Minecraft_1_7_2))
	Login.ClientBound.Register(&p.EncryptionRequest{},
		m(0x01, version.Minecraft_1_7_2))
	Login.ClientBound.Register(&p.ServerLoginSuccess{},
		m(0x02, version.Minecraft_1_7_2))
	Login.ClientBound.Register(&p.SetCompression{},
		m(0x03, version.Minecraft_1_8))
	Login.ClientBound.Register(&p.LoginPluginMessage{},
		m(0x04, version.Minecraft_1_13))

	Play.ServerBound.Fallback = false
	Play.ClientBound.Fallback = false

	Play.ServerBound.Register(&p.KeepAlive{},
		m(0x00, version.Minecraft_1_7_2),
		m(0x0B, version.Minecraft_1_9),
		m(0x0C, version.Minecraft_1_12),
		m(0x0B, version.Minecraft_1_12_1),
		m(0x0E, version.Minecraft_1_13),
		m(0x0F, version.Minecraft_1_14),
		m(0x10, version.Minecraft_1_16),
	)
	Play.ServerBound.Register(&plugin.Message{},
		m(0x17, version.Minecraft_1_7_2),
		m(0x09, version.Minecraft_1_9),
		m(0x0A, version.Minecraft_1_12),
		m(0x09, version.Minecraft_1_12_1),
		m(0x0A, version.Minecraft_1_13),
		m(0x0B, version.Minecraft_1_14),
	)
	Play.ServerBound.Register(&p.ClientSettings{},
		m(0x15, version.Minecraft_1_7_2),
		m(0x04, version.Minecraft_1_9),
		m(0x05, version.Minecraft_1_12),
		m(0x04, version.Minecraft_1_12_1),
		m(0x05, version.Minecraft_1_14),
	)
	Play.ServerBound.Register(&p.Chat{},
		m(0x01, version.Minecraft_1_7_2),
		m(0x02, version.Minecraft_1_9),
		m(0x03, version.Minecraft_1_12),
		m(0x02, version.Minecraft_1_12_1),
		m(0x03, version.Minecraft_1_14),
	)
	// coming soon...
	// TabCompleteRequest
	// ResourcePackResponse

	Play.ClientBound.Register(&p.KeepAlive{},
		m(0x00, version.Minecraft_1_7_2),
		m(0x1F, version.Minecraft_1_9),
		m(0x21, version.Minecraft_1_13),
		m(0x20, version.Minecraft_1_14),
		m(0x21, version.Minecraft_1_15),
		m(0x20, version.Minecraft_1_16),
		m(0x1F, version.Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.JoinGame{},
		m(0x01, version.Minecraft_1_7_2),
		m(0x23, version.Minecraft_1_9),
		m(0x25, version.Minecraft_1_13),
		m(0x25, version.Minecraft_1_14),
		m(0x26, version.Minecraft_1_15),
		m(0x25, version.Minecraft_1_16),
		m(0x24, version.Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.Respawn{},
		m(0x07, version.Minecraft_1_7_2),
		m(0x33, version.Minecraft_1_9),
		m(0x34, version.Minecraft_1_12),
		m(0x35, version.Minecraft_1_12_1),
		m(0x38, version.Minecraft_1_13),
		m(0x3A, version.Minecraft_1_14),
		m(0x3B, version.Minecraft_1_15),
		m(0x3A, version.Minecraft_1_16),
		m(0x39, version.Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.Disconnect{},
		m(0x40, version.Minecraft_1_7_2),
		m(0x1A, version.Minecraft_1_9),
		m(0x1B, version.Minecraft_1_13),
		m(0x1A, version.Minecraft_1_14),
		m(0x1B, version.Minecraft_1_15),
		m(0x1A, version.Minecraft_1_16),
		m(0x19, version.Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.Chat{},
		m(0x02, version.Minecraft_1_7_2),
		m(0x0F, version.Minecraft_1_9),
		m(0x0E, version.Minecraft_1_13),
		m(0x0F, version.Minecraft_1_15),
		m(0x0E, version.Minecraft_1_16),
	)
	Play.ClientBound.Register(&p.HeaderAndFooter{},
		m(0x47, version.Minecraft_1_8),
		m(0x48, version.Minecraft_1_9),
		m(0x47, version.Minecraft_1_9_4),
		m(0x49, version.Minecraft_1_12),
		m(0x4A, version.Minecraft_1_12_1),
		m(0x4E, version.Minecraft_1_13),
		m(0x53, version.Minecraft_1_14),
		m(0x54, version.Minecraft_1_15),
		m(0x53, version.Minecraft_1_16),
	)
	Play.ClientBound.Register(&p.PlayerListItem{},
		m(0x38, version.Minecraft_1_7_2),
		m(0x2D, version.Minecraft_1_9),
		m(0x2E, version.Minecraft_1_12_1),
		m(0x30, version.Minecraft_1_13),
		m(0x33, version.Minecraft_1_14),
		m(0x34, version.Minecraft_1_15),
		m(0x33, version.Minecraft_1_16),
		m(0x32, version.Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.Title{},
		m(0x45, version.Minecraft_1_8),
		m(0x45, version.Minecraft_1_9),
		m(0x47, version.Minecraft_1_12),
		m(0x48, version.Minecraft_1_12_1),
		m(0x4B, version.Minecraft_1_13),
		m(0x4F, version.Minecraft_1_14),
		m(0x50, version.Minecraft_1_15),
		m(0x4F, version.Minecraft_1_16),
	)
	Play.ClientBound.Register(&plugin.Message{},
		m(0x3F, version.Minecraft_1_7_2),
		m(0x18, version.Minecraft_1_9),
		m(0x19, version.Minecraft_1_13),
		m(0x18, version.Minecraft_1_14),
		m(0x19, version.Minecraft_1_15),
		m(0x18, version.Minecraft_1_16),
		m(0x17, version.Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.ResourcePackRequest{},
		m(0x48, version.Minecraft_1_8),
		m(0x32, version.Minecraft_1_9),
		m(0x33, version.Minecraft_1_12),
		m(0x34, version.Minecraft_1_12_1),
		m(0x37, version.Minecraft_1_13),
		m(0x39, version.Minecraft_1_14),
		m(0x3A, version.Minecraft_1_15),
		m(0x39, version.Minecraft_1_16),
		m(0x38, version.Minecraft_1_16_2),
	)
	// coming soon...
	// BossBar
	// TabCompleteResponse
	// AvailableCommands
	// HeaderAndFooter
	// PlayerListItem
}
