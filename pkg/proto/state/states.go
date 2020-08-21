package state

import (
	. "go.minekube.com/gate/pkg/proto"
	p "go.minekube.com/gate/pkg/proto/packet"
	"go.minekube.com/gate/pkg/proto/packet/plugin"
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
		m(0x00, Minecraft_1_7_2))

	Status.ServerBound.Register(&p.StatusRequest{},
		m(0x00, Minecraft_1_7_2))
	Status.ServerBound.Register(&p.StatusPing{},
		m(0x01, Minecraft_1_7_2))

	Status.ClientBound.Register(&p.StatusResponse{},
		m(0x00, Minecraft_1_7_2))
	Status.ClientBound.Register(&p.StatusPing{},
		m(0x01, Minecraft_1_7_2))

	Login.ServerBound.Register(&p.ServerLogin{},
		m(0x00, Minecraft_1_7_2))
	Login.ServerBound.Register(&p.EncryptionResponse{},
		m(0x01, Minecraft_1_7_2))
	Login.ServerBound.Register(&p.LoginPluginResponse{},
		m(0x02, Minecraft_1_7_2))

	Login.ClientBound.Register(&p.Disconnect{},
		m(0x00, Minecraft_1_7_2))
	Login.ClientBound.Register(&p.EncryptionRequest{},
		m(0x01, Minecraft_1_7_2))
	Login.ClientBound.Register(&p.ServerLoginSuccess{},
		m(0x02, Minecraft_1_7_2))
	Login.ClientBound.Register(&p.SetCompression{},
		m(0x03, Minecraft_1_8))
	Login.ClientBound.Register(&p.LoginPluginMessage{},
		m(0x04, Minecraft_1_13))

	Play.ServerBound.Fallback = false
	Play.ClientBound.Fallback = false

	Play.ServerBound.Register(&p.KeepAlive{},
		m(0x00, Minecraft_1_7_2),
		m(0x0B, Minecraft_1_9),
		m(0x0C, Minecraft_1_12),
		m(0x0B, Minecraft_1_12_1),
		m(0x0E, Minecraft_1_13),
		m(0x0F, Minecraft_1_14),
		m(0x10, Minecraft_1_16),
	)
	Play.ServerBound.Register(&plugin.Message{},
		m(0x17, Minecraft_1_7_2),
		m(0x09, Minecraft_1_9),
		m(0x0A, Minecraft_1_12),
		m(0x09, Minecraft_1_12_1),
		m(0x0A, Minecraft_1_13),
		m(0x0B, Minecraft_1_14),
	)
	Play.ServerBound.Register(&p.ClientSettings{},
		m(0x15, Minecraft_1_7_2),
		m(0x04, Minecraft_1_9),
		m(0x05, Minecraft_1_12),
		m(0x04, Minecraft_1_12_1),
		m(0x05, Minecraft_1_14),
	)
	Play.ServerBound.Register(&p.Chat{},
		m(0x01, Minecraft_1_7_2),
		m(0x02, Minecraft_1_9),
		m(0x03, Minecraft_1_12),
		m(0x02, Minecraft_1_12_1),
		m(0x03, Minecraft_1_14),
	)
	// coming soon...
	// TabCompleteRequest
	// ResourcePackResponse

	Play.ClientBound.Register(&p.KeepAlive{},
		m(0x00, Minecraft_1_7_2),
		m(0x1F, Minecraft_1_9),
		m(0x21, Minecraft_1_13),
		m(0x20, Minecraft_1_14),
		m(0x21, Minecraft_1_15),
		m(0x20, Minecraft_1_16),
		m(0x1F, Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.JoinGame{},
		m(0x01, Minecraft_1_7_2),
		m(0x23, Minecraft_1_9),
		m(0x25, Minecraft_1_13),
		m(0x25, Minecraft_1_14),
		m(0x26, Minecraft_1_15),
		m(0x25, Minecraft_1_16),
		m(0x24, Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.Respawn{},
		m(0x07, Minecraft_1_7_2),
		m(0x33, Minecraft_1_9),
		m(0x34, Minecraft_1_12),
		m(0x35, Minecraft_1_12_1),
		m(0x38, Minecraft_1_13),
		m(0x3A, Minecraft_1_14),
		m(0x3B, Minecraft_1_15),
		m(0x3A, Minecraft_1_16),
		m(0x39, Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.Disconnect{},
		m(0x40, Minecraft_1_7_2),
		m(0x1A, Minecraft_1_9),
		m(0x1B, Minecraft_1_13),
		m(0x1A, Minecraft_1_14),
		m(0x1B, Minecraft_1_15),
		m(0x1A, Minecraft_1_16),
		m(0x19, Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.Chat{},
		m(0x02, Minecraft_1_7_2),
		m(0x0F, Minecraft_1_9),
		m(0x0E, Minecraft_1_13),
		m(0x0F, Minecraft_1_15),
		m(0x0E, Minecraft_1_16),
	)
	Play.ClientBound.Register(&p.Title{},
		m(0x45, Minecraft_1_8),
		m(0x45, Minecraft_1_9),
		m(0x47, Minecraft_1_12),
		m(0x48, Minecraft_1_12_1),
		m(0x4B, Minecraft_1_13),
		m(0x4F, Minecraft_1_14),
		m(0x50, Minecraft_1_15),
		m(0x4F, Minecraft_1_16),
	)
	Play.ClientBound.Register(&plugin.Message{},
		m(0x3F, Minecraft_1_7_2),
		m(0x18, Minecraft_1_9),
		m(0x19, Minecraft_1_13),
		m(0x18, Minecraft_1_14),
		m(0x19, Minecraft_1_15),
		m(0x18, Minecraft_1_16),
		m(0x17, Minecraft_1_16_2),
	)
	Play.ClientBound.Register(&p.ResourcePackRequest{},
		m(0x48, Minecraft_1_8),
		m(0x32, Minecraft_1_9),
		m(0x33, Minecraft_1_12),
		m(0x34, Minecraft_1_12_1),
		m(0x37, Minecraft_1_13),
		m(0x39, Minecraft_1_14),
		m(0x3A, Minecraft_1_15),
		m(0x39, Minecraft_1_16),
		m(0x38, Minecraft_1_16_2),
	)
	// coming soon...
	// BossBar
	// TabCompleteResponse
	// AvailableCommands
	// HeaderAndFooter
	// PlayerListItem
}
