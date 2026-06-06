package proxy

import (
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
)

func TestHandshakeConnectionType(t *testing.T) {
	tests := []struct {
		name     string
		h        *packet.Handshake
		wantType phase.ConnectionType
	}{
		{
			name: "vanilla client",
			h: &packet.Handshake{
				ServerAddress:   "server.example.com",
				ProtocolVersion: int(version.Minecraft_1_20_2.Protocol),
			},
			wantType: phase.Vanilla,
		},
		{
			name: "modern forge 1.20.2+ (FORGE token)",
			h: &packet.Handshake{
				ServerAddress:   "server.example.com\000FORGE",
				ProtocolVersion: int(version.Minecraft_1_20_2.Protocol),
			},
			wantType: phase.ModernForge,
		},
		{
			name: "FML2 forge 1.13 client",
			h: &packet.Handshake{
				ServerAddress:   "server.example.com\000FML2\000",
				ProtocolVersion: int(version.Minecraft_1_13.Protocol),
			},
			wantType: phase.ModernForge,
		},
		{
			name: "FML3 forge 1.20.1 client",
			h: &packet.Handshake{
				ServerAddress:   "server.example.com\000FML3\000",
				ProtocolVersion: int(version.Minecraft_1_20.Protocol),
			},
			wantType: phase.ModernForge,
		},
		{
			name: "legacy forge 1.12 client",
			h: &packet.Handshake{
				ServerAddress:   "server.example.com\000FML\000",
				ProtocolVersion: int(version.Minecraft_1_12_2.Protocol),
			},
			wantType: phase.LegacyForge,
		},
		{
			name: "1.7 undetermined",
			h: &packet.Handshake{
				ServerAddress:   "server.example.com",
				ProtocolVersion: int(version.Minecraft_1_7_6.Protocol),
			},
			wantType: phase.Undetermined17,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handshakeConnectionType(tt.h)
			// Use pointer identity (not DeepEqual) because some connection types
			// have identical struct fields (e.g. Vanilla and ModernForge).
			if got != tt.wantType {
				t.Errorf("handshakeConnectionType() = %T(%p), want %T(%p)",
					got, got, tt.wantType, tt.wantType)
			}
		})
	}
}
