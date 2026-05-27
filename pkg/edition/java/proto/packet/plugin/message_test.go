package plugin

import (
	"bytes"
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// buildModernMessage encodes a 1.8+ plugin message frame (channel + raw data).
func buildModernMessage(t *testing.T, channel string, dataLen int) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	if err := util.WriteString(buf, channel); err != nil {
		t.Fatal(err)
	}
	buf.Write(bytes.Repeat([]byte{0x7f}, dataLen))
	return buf
}

func decodeMessage(t *testing.T, dir proto.Direction, data *bytes.Buffer) error {
	t.Helper()
	var m Message
	ctx := &proto.PacketContext{Direction: dir, Protocol: version.Minecraft_1_21.Protocol}
	return m.Decode(ctx, data)
}

// A serverbound plugin message larger than the vanilla 32767-byte limit must be
// rejected so a client cannot abuse it.
func TestDecodeServerboundPluginMessageRejectsOversized(t *testing.T) {
	buf := buildModernMessage(t, "minecraft:brand", 32768)
	if err := decodeMessage(t, proto.ServerBound, buf); err == nil {
		t.Fatal("expected error decoding oversized serverbound plugin message, got nil")
	}
}

// A serverbound plugin message at or below the limit must decode fine.
func TestDecodeServerboundPluginMessageAllowsWithinLimit(t *testing.T) {
	buf := buildModernMessage(t, "minecraft:brand", 32767)
	if err := decodeMessage(t, proto.ServerBound, buf); err != nil {
		t.Fatalf("unexpected error decoding within-limit serverbound plugin message: %v", err)
	}
}

// Clientbound messages (proxy<-backend) may legitimately be large and must not
// be subject to the serverbound limit.
func TestDecodeClientboundPluginMessageAllowsLarge(t *testing.T) {
	buf := buildModernMessage(t, "minecraft:brand", 100000)
	if err := decodeMessage(t, proto.ClientBound, buf); err != nil {
		t.Fatalf("unexpected error decoding large clientbound plugin message: %v", err)
	}
}
