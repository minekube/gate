package floodgate

import (
	"bytes"
	"testing"

	"go.minekube.com/gate/pkg/util/uuid"
)

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	u, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("uuid.Parse(%q): %v", s, err)
	}
	return u
}

func TestParseLinkedPlayer(t *testing.T) {
	javaUUID := mustParseUUID(t, "11111111-2222-3333-4444-555555555555")
	bedrockUUID := mustParseUUID(t, "00000000-0000-0000-0000-00003ADE68B1")

	tests := []struct {
		name string
		raw  string
		want *LinkedPlayer
	}{
		{
			name: "valid triplet",
			raw:  "JavaSteve;11111111-2222-3333-4444-555555555555;00000000-0000-0000-0000-00003ADE68B1",
			want: &LinkedPlayer{JavaUsername: "JavaSteve", JavaUUID: javaUUID, BedrockUUID: bedrockUUID},
		},
		{
			name: "empty string means absent link",
			raw:  "",
			want: nil,
		},
		{
			name: "literal null means absent link",
			raw:  "null",
			want: nil,
		},
		{
			name: "two parts is malformed",
			raw:  "JavaSteve;11111111-2222-3333-4444-555555555555",
			want: nil,
		},
		{
			name: "four parts is malformed",
			raw:  "JavaSteve;11111111-2222-3333-4444-555555555555;00000000-0000-0000-0000-00003ADE68B1;extra",
			want: nil,
		},
		{
			name: "invalid java uuid is malformed",
			raw:  "JavaSteve;not-a-uuid;00000000-0000-0000-0000-00003ADE68B1",
			want: nil,
		},
		{
			name: "invalid bedrock uuid is malformed",
			raw:  "JavaSteve;11111111-2222-3333-4444-555555555555;not-a-uuid",
			want: nil,
		},
		{
			name: "empty java username is malformed",
			raw:  ";11111111-2222-3333-4444-555555555555;00000000-0000-0000-0000-00003ADE68B1",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLinkedPlayer(tt.raw)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("ParseLinkedPlayer(%q) = %#v, want nil", tt.raw, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ParseLinkedPlayer(%q) = nil, want %#v", tt.raw, tt.want)
			}
			if *got != *tt.want {
				t.Fatalf("ParseLinkedPlayer(%q) = %#v, want %#v", tt.raw, *got, *tt.want)
			}
		})
	}
}

func TestFloodgateJavaUuid(t *testing.T) {
	// Matches Floodgate's Utils.getJavaUuid: new UUID(0, xuid).
	tests := []struct {
		xuid int64
		want string
	}{
		{xuid: 987654321, want: "00000000-0000-0000-0000-00003ade68b1"},
		{xuid: 123456789, want: "00000000-0000-0000-0000-0000075bcd15"},
		{xuid: 281474976710655, want: "00000000-0000-0000-0000-ffffffffffff"},
		{xuid: 0, want: "00000000-0000-0000-0000-000000000000"},
	}
	for _, tt := range tests {
		got := (&BedrockData{Xuid: tt.xuid}).FloodgateJavaUuid()
		if got.String() != tt.want {
			t.Errorf("FloodgateJavaUuid(%d) = %s, want %s", tt.xuid, got, tt.want)
		}
	}
}

func TestLinkedPlayerRoundTripsThroughWriteHostname(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 16)
	cfg, err := NewFloodgate(key)
	if err != nil {
		t.Fatalf("NewFloodgate: %v", err)
	}

	want := &BedrockData{
		Version:      "1",
		Username:     "Fox",
		Xuid:         123456789,
		DeviceOS:     DeviceOSWindowsUWP,
		Language:     "en_US",
		UIProfile:    1,
		InputMode:    2,
		IP:           "203.0.113.10",
		LinkedPlayer: "JavaSteve;11111111-2222-3333-4444-555555555555;00000000-0000-0000-0000-0000075BCD15",
		Proxy:        true,
		SubscribeID:  "sub-id",
		VerifyCode:   "verify-code",
	}

	host, err := cfg.WriteHostname("play.example.org:19132", want)
	if err != nil {
		t.Fatalf("WriteHostname: %v", err)
	}

	_, got, err := cfg.ReadHostname(host)
	if err != nil {
		t.Fatalf("ReadHostname: %v", err)
	}

	// The canonical serialized form must survive the round trip unchanged.
	if got.LinkedPlayer != want.LinkedPlayer {
		t.Fatalf("LinkedPlayer = %q, want %q", got.LinkedPlayer, want.LinkedPlayer)
	}
	link := ParseLinkedPlayer(got.LinkedPlayer)
	if link == nil {
		t.Fatalf("ParseLinkedPlayer(%q) = nil after round trip", got.LinkedPlayer)
	}
	if link.JavaUsername != "JavaSteve" {
		t.Fatalf("JavaUsername = %q, want JavaSteve", link.JavaUsername)
	}
}
