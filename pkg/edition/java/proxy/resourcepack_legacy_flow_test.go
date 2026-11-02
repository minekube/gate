package proxy

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/internal/resourcepack"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/gate/proto"
)

type resourcePackFlowConn struct {
	testMinecraftConn
	rawWrites int
}

func (c *resourcePackFlowConn) Write([]byte) error {
	c.rawWrites++
	return nil
}

func runResourcePackFlow(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("resource pack session handler timed out")
	}
}

func TestLegacyResourcePackPlayFlow(t *testing.T) {
	for _, required := range []bool{false, true} {
		t.Run(map[bool]string{false: "optional", true: "required"}[required], func(t *testing.T) {
			client := &resourcePackFlowConn{testMinecraftConn: testMinecraftConn{protocol: version.Minecraft_1_20.Protocol}}
			original := &resourcePackFlowConn{testMinecraftConn: testMinecraftConn{protocol: version.Minecraft_1_20.Protocol}}
			current := &resourcePackFlowConn{testMinecraftConn: testMinecraftConn{protocol: version.Minecraft_1_20.Protocol}}
			player := &connectedPlayer{MinecraftConn: client, log: logr.Discard()}
			player.sessionHandlerDeps = &sessionHandlerDeps{proxy: &Proxy{event: event.Nop}}
			player.resourcePackHandler = resourcepack.NewHandler(player, event.Nop)
			originalServer := &serverConnection{player: player, connection: original, connPhase: phase.VanillaBackendPhase, log: logr.Discard()}
			currentServer := &serverConnection{player: player, connection: current, connPhase: phase.VanillaBackendPhase, log: logr.Discard()}
			player.connectedServer_ = currentServer
			player.connInFlight = currentServer
			backendHandler := &backendPlaySessionHandler{serverConn: originalServer, log: logr.Discard()}
			clientHandler := &clientPlaySessionHandler{player: player, log: logr.Discard()}
			request := &packet.ResourcePackRequest{URL: "https://example.invalid/pack.zip", Required: required}

			runResourcePackFlow(t, func() { backendHandler.HandlePacket(&proto.PacketContext{Packet: request}) })
			if len(client.writtenPackets) != 1 {
				t.Fatalf("client packets = %d, want 1", len(client.writtenPackets))
			}
			got, ok := client.writtenPackets[0].(*packet.ResourcePackRequest)
			if !ok || got.URL != request.URL || got.Required != required {
				t.Fatalf("client request = %#v", client.writtenPackets[0])
			}
			for _, status := range []packet.ResponseStatus{packet.AcceptedResourcePackResponseStatus, packet.SuccessfulResourcePackResponseStatus} {
				response := &packet.ResourcePackResponse{Status: status}
				runResourcePackFlow(t, func() {
					clientHandler.HandlePacket(&proto.PacketContext{Packet: response, Payload: []byte{byte(status)}})
				})
			}
			if len(original.writtenPackets) != 2 {
				t.Fatalf("origin responses = %d, want 2", len(original.writtenPackets))
			}
			for i, want := range []packet.ResponseStatus{packet.AcceptedResourcePackResponseStatus, packet.SuccessfulResourcePackResponseStatus} {
				response, ok := original.writtenPackets[i].(*packet.ResourcePackResponse)
				if !ok || response.Status != want {
					t.Fatalf("origin response %d = %#v, want status %v", i, original.writtenPackets[i], want)
				}
			}
			// A late response after the tracked request completes has no safe recipient.
			runResourcePackFlow(t, func() {
				clientHandler.HandlePacket(&proto.PacketContext{
					Packet:  &packet.ResourcePackResponse{Status: packet.SuccessfulResourcePackResponseStatus},
					Payload: []byte{0},
				})
			})
			if len(original.writtenPackets) != 2 || len(current.writtenPackets) != 0 || current.rawWrites != 0 || original.rawWrites != 0 {
				t.Fatalf("response forwarded to the wrong or duplicate connection: origin raw=%d, current packets=%d raw=%d",
					original.rawWrites, len(current.writtenPackets), current.rawWrites)
			}
		})
	}
}
