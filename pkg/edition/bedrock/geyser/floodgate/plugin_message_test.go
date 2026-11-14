package floodgate

import (
	"testing"

	"go.minekube.com/gate/pkg/util/uuid"
)

// Floodgate resolves a session by the UUID the proxy forwarded. The only
// identity it never derives is Gate's XUID-derived v5 UUID, which is what an
// unlinked Bedrock player reaches a Velocity-forwarding backend under.
func TestResolvesProfileUUID(t *testing.T) {
	data := &BedrockData{Username: "BedrockPlayer", Xuid: 987654321}
	xuidIdentity, err := data.JavaUuid()
	if err != nil {
		t.Fatalf("JavaUuid: %v", err)
	}

	cases := []struct {
		name string
		id   uuid.UUID
		want bool
	}{
		{
			name: "Floodgate's own unlinked UUID is its registry key",
			id:   data.FloodgateJavaUuid(),
			want: true,
		},
		{
			name: "Gate's XUID-derived v5 UUID is one Floodgate cannot know",
			id:   xuidIdentity,
			want: false,
		},
		{
			name: "a linked Java UUID is resolved by getCorrectUniqueId",
			id:   uuid.OfflinePlayerUUID("LinkedJava"),
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := data.ResolvesProfileUUID(tc.id); got != tc.want {
				t.Fatalf("ResolvesProfileUUID(%s) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestResolvesProfileUUIDFailsOpenOnNilData(t *testing.T) {
	var data *BedrockData
	if !data.ResolvesProfileUUID(uuid.OfflinePlayerUUID("anyone")) {
		t.Fatal("nil BedrockData must report resolvable: the predicate gates dropping a message")
	}
}

func TestIsClientChannel(t *testing.T) {
	for _, channel := range []string{SkinChannel, FormChannel, TransferChannel, PacketChannel} {
		if !IsClientChannel(channel) {
			t.Fatalf("IsClientChannel(%q) = false, want true", channel)
		}
	}
	for _, channel := range []string{"minecraft:brand", "floodgate", "bungeecord:main", ""} {
		if IsClientChannel(channel) {
			t.Fatalf("IsClientChannel(%q) = true, want false", channel)
		}
	}
}
