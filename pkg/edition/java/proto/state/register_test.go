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
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	proto "go.minekube.com/gate/pkg/gate/proto"
)

// TestPluginMessagePacketID_1_21_9 verifies that the Play ClientBound plugin.Message
// packet ID is correct for Minecraft 1.21.9 (protocol 773).
// This was incorrectly mapped to 0x1C instead of 0x18, causing BungeeCord
// plugin channel messages to be silently dropped (fixes #612).
func TestPluginMessagePacketID_1_21_9(t *testing.T) {
	tests := []struct {
		name     string
		protocol proto.Protocol
		wantID   int
	}{
		{"1.21.5 should be 0x18", version.Minecraft_1_21_5.Protocol, 0x18},
		{"1.21.7 should be 0x18", version.Minecraft_1_21_7.Protocol, 0x18},
		{"1.21.9 should be 0x18", version.Minecraft_1_21_9.Protocol, 0x18},
		{"1.21.11 should be 0x18", version.Minecraft_1_21_11.Protocol, 0x18},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := Play.ClientBound.ProtocolRegistry(tc.protocol)
			require.NotNil(t, reg, "no protocol registry for protocol %d", tc.protocol)
			id, ok := reg.PacketID(&plugin.Message{})
			require.True(t, ok, "plugin.Message not registered for protocol %d", tc.protocol)
			require.Equal(t, proto.PacketID(tc.wantID), id,
				"plugin.Message packet ID mismatch for protocol %d", tc.protocol)
		})
	}
}

// TestPacketIDs_26_1 verifies that packet IDs are correctly mapped for Minecraft 26.1 (protocol 775).
func TestPacketIDs_26_1(t *testing.T) {
	v := version.Minecraft_26_1.Protocol

	// Play ServerBound
	sbTests := []struct {
		name   string
		packet proto.Packet
		wantID int
	}{
		{"KeepAlive", &p.KeepAlive{}, 0x1C},
		{"PluginMessage", &plugin.Message{}, 0x16},
		{"ClientSettings", &p.ClientSettings{}, 0x0E},
		{"ChatAcknowledgement", &chat.ChatAcknowledgement{}, 0x06},
		{"SessionPlayerCommand", &chat.SessionPlayerCommand{}, 0x08},
		{"UnsignedPlayerCommand", &chat.UnsignedPlayerCommand{}, 0x07},
		{"SessionPlayerChat", &chat.SessionPlayerChat{}, 0x09},
		{"TabCompleteRequest", &p.TabCompleteRequest{}, 0x0F},
		{"ResourcePackResponse", &p.ResourcePackResponse{}, 0x31},
		{"FinishedUpdate", &config.FinishedUpdate{}, 0x10},
		{"CookieResponse", &cookie.CookieResponse{}, 0x15},
	}
	for _, tc := range sbTests {
		t.Run("ServerBound/"+tc.name, func(t *testing.T) {
			reg := Play.ServerBound.ProtocolRegistry(v)
			require.NotNil(t, reg)
			id, ok := reg.PacketID(tc.packet)
			require.True(t, ok, "%s not registered", tc.name)
			require.Equal(t, proto.PacketID(tc.wantID), id)
		})
	}

	// Play ClientBound
	cbTests := []struct {
		name   string
		packet proto.Packet
		wantID int
	}{
		{"KeepAlive", &p.KeepAlive{}, 0x2C},
		{"JoinGame", &p.JoinGame{}, 0x31},
		{"Respawn", &p.Respawn{}, 0x52},
		{"RemoveResourcePack", &p.RemoveResourcePack{}, 0x50},
		{"ResourcePackRequest", &p.ResourcePackRequest{}, 0x51},
		{"HeaderAndFooter", &p.HeaderAndFooter{}, 0x7A},
		{"RemovePlayerInfo", &playerinfo.Remove{}, 0x45},
		{"UpsertPlayerInfo", &playerinfo.Upsert{}, 0x46},
		{"SystemChat", &chat.SystemChat{}, 0x79},
		{"ServerData", &p.ServerData{}, 0x56},
		{"StartUpdate", &config.StartUpdate{}, 0x76},
		{"Transfer", &p.Transfer{}, 0x81},
		{"CustomReportDetails", &p.CustomReportDetails{}, 0x88},
		{"ServerLinks", &p.ServerLinks{}, 0x89},
		{"SoundEntity", &p.SoundEntityPacket{}, 0x74},
		{"StopSound", &p.StopSoundPacket{}, 0x77},
		{"CookieStore", &cookie.CookieStore{}, 0x78},
	}
	for _, tc := range cbTests {
		t.Run("ClientBound/"+tc.name, func(t *testing.T) {
			reg := Play.ClientBound.ProtocolRegistry(v)
			require.NotNil(t, reg)
			id, ok := reg.PacketID(tc.packet)
			require.True(t, ok, "%s not registered", tc.name)
			require.Equal(t, proto.PacketID(tc.wantID), id)
		})
	}
}
