package proxy

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/bungeecord"
	"go.minekube.com/gate/pkg/gate/proto"
)

// TestServerDataMotdStyleSurvivesCodecForClientVersion pins the server_data
// MOTD the proxy synthesizes for a joining player
// (session_backend_play.go handleServerData). The MOTD does not come from the
// backend packet: it is rebuilt from the endpoint's ping, i.e. from a
// Go component.Component, so the component codec version decides which keys go
// on the wire.
//
// 1.21.5 renamed the style keys to snake_case (hover_event/click_event).
// Vanilla's component codec silently ignores unknown keys, so an MOTD encoded
// with the pre-1.21.5 (camelCase) codec keeps its text but loses its hover
// text, click action and shadow_color on 1.21.5+ clients. A zero-valued
// ComponentHolder.Protocol therefore means "encode as pre-1.16", regardless of
// what the client can read.
//
// Both sides of the 1.21.5 boundary are covered because the codec change
// applies to every component that goes through handleServerData.
func TestServerDataMotdStyleSurvivesCodecForClientVersion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol proto.Protocol
		hoverKey string
		clickKey string
		// staleKeys must not appear: they are the other side of the 1.21.5
		// rename and would be ignored by the client reading this protocol.
		staleKeys []string
	}{
		{
			name:     "1.20.4",
			protocol: version.Minecraft_1_20_3.Protocol,
			hoverKey: "hoverEvent",
			clickKey: "clickEvent",
			// 1.21.5+ keys are unknown to a 1.20.4 client.
			staleKeys: []string{"hover_event", "click_event"},
		},
		{
			name:      "1.21.5",
			protocol:  version.Minecraft_1_21_5.Protocol,
			hoverKey:  "hover_event",
			clickKey:  "click_event",
			staleKeys: []string{"hoverEvent", "clickEvent"},
		},
		{
			name:      "26.3",
			protocol:  version.Minecraft_26_3.Protocol,
			hoverKey:  "hover_event",
			clickKey:  "click_event",
			staleKeys: []string{"hoverEvent", "clickEvent"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			written := serverDataWrittenForMotd(t, tc.protocol)
			onWire := encodeDecodeServerData(t, tc.protocol, written)
			if onWire.Description == nil {
				t.Fatal("server_data carried no description")
			}

			raw, err := onWire.Description.AsJson()
			if err != nil {
				t.Fatalf("description as json: %v", err)
			}
			var style map[string]any
			if err := json.Unmarshal(raw, &style); err != nil {
				t.Fatalf("unmarshal description %s: %v", raw, err)
			}
			for _, key := range []string{tc.hoverKey, tc.clickKey} {
				if _, ok := style[key]; !ok {
					t.Fatalf("%s: MOTD lost %q on the wire: %s", tc.name, key, raw)
				}
			}
			for _, key := range tc.staleKeys {
				if _, ok := style[key]; ok {
					t.Fatalf("%s: MOTD used %q, which this client version ignores: %s", tc.name, key, raw)
				}
			}

			hover, _ := style[tc.hoverKey].(map[string]any)
			if hover == nil {
				t.Fatalf("%s: %q is not an object: %s", tc.name, tc.hoverKey, raw)
			}
			if action, _ := hover["action"].(string); action != "show_text" {
				t.Fatalf("%s: hover action = %q, want show_text: %s", tc.name, action, raw)
			}
			if !bytes.Contains(raw, []byte("answer")) {
				t.Fatalf("%s: hover text lost: %s", tc.name, raw)
			}
			if !bytes.Contains(raw, []byte("https://example.com")) {
				t.Fatalf("%s: click action lost: %s", tc.name, raw)
			}
			// 1.21.5 added shadow_color; only the modern codec emits it.
			if tc.protocol.GreaterEqual(version.Minecraft_1_21_5) {
				if _, ok := style["shadow_color"]; !ok {
					t.Fatalf("%s: MOTD lost shadow_color: %s", tc.name, raw)
				}
			}
		})
	}
}

// serverDataWrittenForMotd drives the production handler
// (handleServerData -> PingEvent -> write) with a styled endpoint MOTD and
// returns the packet the proxy wrote to the player.
func serverDataWrittenForMotd(t *testing.T, protocol proto.Protocol) *packet.ServerData {
	t.Helper()

	conn := &testMinecraftConn{protocol: protocol}
	eventMgr := event.New()
	proxy := &Proxy{cfg: &config.Config{}, event: eventMgr}
	player := &connectedPlayer{MinecraftConn: conn, log: logr.Discard()}
	player.sessionHandlerDeps = &sessionHandlerDeps{proxy: proxy}
	serverConn := &serverConnection{player: player, log: logr.Discard()}

	event.Subscribe(eventMgr, 0, func(e *PingEvent) {
		e.SetPing(&ping.ServerPing{Description: styledMotd()})
	})

	handler := &backendPlaySessionHandler{
		serverConn:                 serverConn,
		bungeeCordMessageResponder: bungeecord.NopMessageResponder,
		log:                        logr.Discard(),
	}
	handler.handleServerData(&packet.ServerData{})
	eventMgr.Wait() // handleServerData fires PingEvent in a goroutine

	if len(conn.writtenPackets) != 1 {
		t.Fatalf("proxy wrote %d packets to the player, want 1 server_data", len(conn.writtenPackets))
	}
	sd, ok := conn.writtenPackets[0].(*packet.ServerData)
	if !ok {
		t.Fatalf("proxy wrote %T, want *packet.ServerData", conn.writtenPackets[0])
	}
	return sd
}

// encodeDecodeServerData round-trips the packet through its wire form so the
// assertions see exactly the keys a client of that protocol parses.
func encodeDecodeServerData(t *testing.T, protocol proto.Protocol, sd *packet.ServerData) *packet.ServerData {
	t.Helper()

	buf := new(bytes.Buffer)
	if err := sd.Encode(&proto.PacketContext{Direction: proto.ClientBound, Protocol: protocol}, buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded := new(packet.ServerData)
	if err := decoded.Decode(&proto.PacketContext{Direction: proto.ClientBound, Protocol: protocol}, bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded
}

// styledMotd is an endpoint MOTD with the styling the 1.21.5 component codec
// renamed: a hover text, an open_url click action and a shadow color.
func styledMotd() component.Component {
	return &component.Text{
		Content: "q",
		S: component.Style{
			HoverEvent:  component.ShowText(&component.Text{Content: "answer"}),
			ClickEvent:  component.OpenUrl("https://example.com"),
			ShadowColor: component.ShadowColorFromARGB(0x7F000000),
		},
	}
}
