package state

import (
	"testing"

	"github.com/stretchr/testify/require"

	p "go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/playerinfo"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/title"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	proto "go.minekube.com/gate/pkg/gate/proto"
)

// TestPacketIDs_26_3 verifies the packet IDs Gate uses for Minecraft 26.3
// (protocol 777) against the vanilla client's own registration order.
//
// 26.3 inserted three clientbound play packets (AddTransientBlock at 0x25,
// PostEffects at 0x53, SwingAnimation at 0x7b), one serverbound play packet
// (Punch at 0x2e, replacing Swing, whose removal cancels the shift again at
// 0x40) and one clientbound configuration packet (PostEffects at 0x0a). Every
// packet registered after an insertion shifted, so a Gate that only bumps the
// version table and inherits the previous version's IDs mis-reads the whole
// configuration phase and most of the play phase.
//
// Derivation: Mojang client.jar 26.3, classes
// net/minecraft/network/protocol/{login,configuration,game}/*Protocols - the
// registration order of LoginPacketTypes/GamePacketTypes fields is the packet ID
// (validated against the known stable login IDs). The login and configuration
// packet *layouts* are unchanged since 26.2; only these IDs moved.
func TestPacketIDs_26_3(t *testing.T) {
	tests := []struct {
		name     string
		registry *Registry
		dir      proto.Direction
		packet   proto.Packet
		id262    int
		id263    int
	}{
		// Configuration, clientbound: PostEffects inserted at 0x0a shifts everything after it.
		{"config CookieRequest", Config, proto.ClientBound, &cookie.CookieRequest{}, 0x00, 0x00},
		{"config Message", Config, proto.ClientBound, &plugin.Message{}, 0x01, 0x01},
		{"config Disconnect", Config, proto.ClientBound, &p.Disconnect{}, 0x02, 0x02},
		{"config FinishedUpdate", Config, proto.ClientBound, &config.FinishedUpdate{}, 0x03, 0x03},
		{"config KeepAlive", Config, proto.ClientBound, &p.KeepAlive{}, 0x04, 0x04},
		{"config RegistrySync", Config, proto.ClientBound, &config.RegistrySync{}, 0x07, 0x07},
		{"config CookieStore", Config, proto.ClientBound, &cookie.CookieStore{}, 0x0A, 0x0B},
		{"config Transfer", Config, proto.ClientBound, &p.Transfer{}, 0x0B, 0x0C},
		{"config ActiveFeatures", Config, proto.ClientBound, &config.ActiveFeatures{}, 0x0C, 0x0D},
		{"config TagsUpdate", Config, proto.ClientBound, &config.TagsUpdate{}, 0x0D, 0x0E},
		{"config KnownPacks", Config, proto.ClientBound, &config.KnownPacks{}, 0x0E, 0x0F},
		{"config CustomReportDetails", Config, proto.ClientBound, &p.CustomReportDetails{}, 0x0F, 0x10},
		{"config ServerLinks", Config, proto.ClientBound, &p.ServerLinks{}, 0x10, 0x11},
		{"config DialogClear", Config, proto.ClientBound, &p.DialogClear{}, 0x11, 0x12},
		{"config DialogShow", Config, proto.ClientBound, &p.DialogShow{}, 0x12, 0x13},
		{"config CodeOfConduct", Config, proto.ClientBound, &config.CodeOfConductPacket{}, 0x13, 0x14},

		// Play, clientbound: AddTransientBlock 0x25, PostEffects 0x53, SwingAnimation 0x7b.
		{"play BundleDelimiter", Play, proto.ClientBound, &p.BundleDelimiter{}, 0x00, 0x00},
		{"play Disconnect", Play, proto.ClientBound, &p.Disconnect{}, 0x20, 0x20},
		{"play KeepAlive", Play, proto.ClientBound, &p.KeepAlive{}, 0x2C, 0x2D},
		{"play JoinGame", Play, proto.ClientBound, &p.JoinGame{}, 0x31, 0x32},
		{"play PlayerInfoRemove", Play, proto.ClientBound, &playerinfo.Remove{}, 0x45, 0x46},
		{"play PlayerInfoUpdate", Play, proto.ClientBound, &playerinfo.Upsert{}, 0x46, 0x47},
		{"play RemoveResourcePack", Play, proto.ClientBound, &p.RemoveResourcePack{}, 0x50, 0x51},
		{"play ResourcePackRequest", Play, proto.ClientBound, &p.ResourcePackRequest{}, 0x51, 0x52},
		{"play Respawn", Play, proto.ClientBound, &p.Respawn{}, 0x52, 0x54},
		{"play ServerData", Play, proto.ClientBound, &p.ServerData{}, 0x56, 0x58},
		{"play Actionbar", Play, proto.ClientBound, &title.Actionbar{}, 0x57, 0x59},
		{"play Subtitle", Play, proto.ClientBound, &title.Subtitle{}, 0x70, 0x72},
		{"play TitleText", Play, proto.ClientBound, &title.Text{}, 0x72, 0x74},
		{"play TitleTimes", Play, proto.ClientBound, &title.Times{}, 0x73, 0x75},
		{"play SoundEntity", Play, proto.ClientBound, &p.SoundEntityPacket{}, 0x74, 0x76},
		{"play StartUpdate", Play, proto.ClientBound, &config.StartUpdate{}, 0x76, 0x78},
		{"play StopSound", Play, proto.ClientBound, &p.StopSoundPacket{}, 0x77, 0x79},
		{"play CookieStore", Play, proto.ClientBound, &cookie.CookieStore{}, 0x78, 0x7A},
		{"play SystemChat", Play, proto.ClientBound, &chat.SystemChat{}, 0x79, 0x7C},
		{"play HeaderAndFooter", Play, proto.ClientBound, &p.HeaderAndFooter{}, 0x7A, 0x7D},
		{"play Transfer", Play, proto.ClientBound, &p.Transfer{}, 0x81, 0x84},
		{"play CustomReportDetails", Play, proto.ClientBound, &p.CustomReportDetails{}, 0x88, 0x8B},
		{"play ServerLinks", Play, proto.ClientBound, &p.ServerLinks{}, 0x89, 0x8C},

		// Play, serverbound: Punch inserted at 0x2e, Swing removed at 0x3f cancels out again at 0x40.
		{"play ChatAcknowledgement", Play, proto.ServerBound, &chat.ChatAcknowledgement{}, 0x06, 0x06},
		{"play SessionPlayerChat", Play, proto.ServerBound, &chat.SessionPlayerChat{}, 0x09, 0x09},
		{"play ClientSettings", Play, proto.ServerBound, &p.ClientSettings{}, 0x0E, 0x0E},
		{"play KeepAlive", Play, proto.ServerBound, &p.KeepAlive{}, 0x1C, 0x1C},
		{"play ResourcePackResponse", Play, proto.ServerBound, &p.ResourcePackResponse{}, 0x31, 0x32},

		// Login: unchanged in 26.3 (verified against the client jar).
		{"login Disconnect", Login, proto.ClientBound, &p.Disconnect{}, 0x00, 0x00},
		{"login EncryptionRequest", Login, proto.ClientBound, &p.EncryptionRequest{}, 0x01, 0x01},
		{"login ServerLoginSuccess", Login, proto.ClientBound, &p.ServerLoginSuccess{}, 0x02, 0x02},
		{"login SetCompression", Login, proto.ClientBound, &p.SetCompression{}, 0x03, 0x03},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range []struct {
				protocol proto.Protocol
				id       int
			}{
				{version.Minecraft_26_2.Protocol, tt.id262},
				{777, tt.id263},
			} {
				registry := protocolRegistry(tt.registry, tt.dir, want.protocol)
				require.NotNil(t, registry, "no %s registry for protocol %d", tt.dir, want.protocol)
				id, ok := registry.PacketID(tt.packet)
				require.True(t, ok, "%T not registered for protocol %d", tt.packet, want.protocol)
				require.Equal(t, proto.PacketID(want.id), id,
					"protocol %d: %T id = %#x, want %#x", want.protocol, tt.packet, id, want.id)
			}
		})
	}
}

func protocolRegistry(r *Registry, dir proto.Direction, protocol proto.Protocol) *ProtocolRegistry {
	if dir == proto.ServerBound {
		return r.ServerBound.ProtocolRegistry(protocol)
	}
	return r.ClientBound.ProtocolRegistry(protocol)
}
