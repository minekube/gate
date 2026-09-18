package state

import (
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// TestProtocolRegistryFallsBackToNearestKnownVersion pins what a registry does
// with a protocol that is not in the version table.
//
// Regression (2026-09-18, Minecraft 26.3 / protocol 777): the fallback always
// picked the *minimum* version, so a client newer than everything Gate knows
// (e.g. 778, 900) had every clientbound packet encoded in the 1.7.2 wire format
// - including the login "hello" (EncryptionRequest), which modern clients then
// fail to decode. The nearest known version is the only sane fallback: below the
// minimum use the minimum, above the maximum use the maximum.
func TestProtocolRegistryFallsBackToNearestKnownVersion(t *testing.T) {
	tests := []struct {
		name     string
		protocol proto.Protocol
		want     proto.Protocol
	}{
		{"known protocol is used as-is", version.Minecraft_26_2.Protocol, version.Minecraft_26_2.Protocol},
		{"newest release is used as-is", 777, 777},
		{"one above the table clamps to the newest known version", 778, version.MaximumVersion.Protocol},
		{"far above the table clamps to the newest known version", 900, version.MaximumVersion.Protocol},
		{"below the minimum falls back to the oldest known version", 3, version.MinimumVersion.Protocol},
		{"legacy falls back to the oldest known version", version.Legacy.Protocol, version.MinimumVersion.Protocol},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, registry := range []*PacketRegistry{Login.ClientBound, Login.ServerBound} {
				got := registry.ProtocolRegistry(tt.protocol)
				if got == nil {
					t.Fatalf("ProtocolRegistry(%d) = nil", tt.protocol)
				}
				if got.Protocol != tt.want {
					t.Errorf("ProtocolRegistry(%d).Protocol = %d, want %d", tt.protocol, got.Protocol, tt.want)
				}
			}
		})
	}
}

// TestProtocolRegistryNoFallbackReturnsNil makes sure the fallback stays opt-out
// for the states that disable it (the play state forwards unknown packets).
func TestProtocolRegistryNoFallbackReturnsNil(t *testing.T) {
	if got := Play.ClientBound.ProtocolRegistry(900); got != nil {
		t.Fatalf("Play.ClientBound.ProtocolRegistry(900) = %v, want nil (Fallback disabled)", got)
	}
}
