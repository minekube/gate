package state

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
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
