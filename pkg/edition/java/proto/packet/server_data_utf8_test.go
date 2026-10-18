package packet

import (
	"encoding/hex"
	"strings"
	"testing"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// TestServerDataEmojiMotdIsJavaReadable pins the packet the Connect edge
// synthesizes for a player (session_backend_play.go handleServerData): the MOTD
// comes from the endpoint's ping, so it can contain emoji or any other character
// outside the BMP. NBT strings are read by the client with
// java.io.DataInputStream.readUTF (modified UTF-8), which has no 4-byte form: an
// astral character written as standard UTF-8 makes the client fail with
// "Failed to decode packet 'clientbound/minecraft:server_data'" and disconnect
// ~1 s after spawn.
func TestServerDataEmojiMotdIsJavaReadable(t *testing.T) {
	motd := &chat.ComponentHolder{Component: &component.Text{Content: "\u00a7cRed \U0001F600"}}

	for name, protocol := range map[string]proto.Protocol{
		"26.2": version.Minecraft_26_2.Protocol,
		"26.3": version.Minecraft_26_3.Protocol,
	} {
		t.Run(name, func(t *testing.T) {
			sd := &ServerData{Description: motd}
			buf := new(writeBuf)
			ctx := &proto.PacketContext{Direction: proto.ClientBound, Protocol: protocol, PacketID: 0x58, Packet: sd}
			if err := sd.Encode(ctx, buf); err != nil {
				t.Fatalf("encode: %v", err)
			}
			// The smiley must be on the wire as the CESU-8 surrogate pair
			// ED A0 BD ED B8 80 (what DataOutputStream.writeUTF writes), never as
			// the 4-byte UTF-8 sequence F0 9F 98 80.
			body := hex.EncodeToString(buf.b)
			if !strings.Contains(body, "eda0bdedb880") {
				t.Fatalf("motd is not modified-UTF-8 encoded; body=%s", body)
			}
			if strings.Contains(body, "f09f9880") {
				t.Fatalf("motd contains a 4-byte UTF-8 sequence a Java client cannot decode; body=%s", body)
			}
		})
	}
}

type writeBuf struct{ b []byte }

func (w *writeBuf) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
