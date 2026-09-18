package version

import (
	"testing"

	"go.minekube.com/gate/pkg/gate/proto"
)

// TestSupportedProtocols covers the version gate that decides whether Gate may
// negotiate a login with a client.
//
// Regression (2026-09-18, Minecraft 26.3 / protocol 777): Supported was
// implemented as "!Unknown()", i.e. it only rejected the -1 sentinel, so *every*
// numeric client protocol - including protocols Gate has never heard of - was
// treated as supported. Login then proceeded and the login packets were encoded
// through the minimum-version fallback registry (pre-1.8 wire format), which a
// modern client cannot decode ("Failed to decode packet
// 'clientbound/minecraft:hello'").
func TestSupportedProtocols(t *testing.T) {
	tests := []struct {
		name     string
		protocol proto.Protocol
		want     bool
	}{
		{"oldest supported 1.7.2", Minecraft_1_7_2.Protocol, true},
		{"1.20.2", Minecraft_1_20_2.Protocol, true},
		{"1.21.11", Minecraft_1_21_11.Protocol, true},
		{"26.2", Minecraft_26_2.Protocol, true},
		{"newest release 26.3", 777, true},

		{"one above the table", 778, false},
		{"far above the table", 900, false},
		{"unknown snapshot inside a gap", 449, false},
		{"below the minimum", 3, false},
		{"unknown sentinel", Unknown.Protocol, false},
		{"legacy sentinel", Legacy.Protocol, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Protocol(tt.protocol).Supported(); got != tt.want {
				t.Errorf("Protocol(%d).Supported() = %t, want %t", tt.protocol, got, tt.want)
			}
		})
	}
}

// TestNewestReleaseIsInVersionTable pins the newest supported Minecraft Java
// release (26.3, protocol 777) into the version table: the status banner, the
// maximum-version fallbacks and the packet-ID registrations all derive from it.
func TestNewestReleaseIsInVersionTable(t *testing.T) {
	const protocol777 = proto.Protocol(777)

	v := Protocol(protocol777).Version()
	if v == Unknown {
		t.Fatalf("protocol 777 (Minecraft 26.3) is missing from the version table")
	}
	if v != MaximumVersion {
		t.Fatalf("Protocol(777).Version() = %s, but MaximumVersion = %s", v, MaximumVersion)
	}
	if len(v.Names) == 0 || v.Names[0] != "26.3" {
		t.Fatalf("Minecraft_26_3 names = %v, want 26.3 first", v.Names)
	}
	if got := SupportedVersionsString; got != "1.7.2-1.7.5-26.3" {
		t.Fatalf("SupportedVersionsString = %q, want %q", got, "1.7.2-1.7.5-26.3")
	}

	// Versions must stay ordered ascending (the two negative sentinels, Unknown and
	// Legacy, are intentionally listed first): Registration walks the slice and
	// panics on an out-of-order protocol.
	last := MinimumVersion.Protocol
	for _, v := range Versions {
		if v.Protocol < MinimumVersion.Protocol {
			continue
		}
		if v.Protocol < last {
			t.Fatalf("Versions is not ordered ascending: %s after %d", v, last)
		}
		last = v.Protocol
	}
}
