package resourcepack

import (
	"testing"

	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type testPlayer struct{}

func (testPlayer) ID() uuid.UUID                          { return uuid.New() }
func (testPlayer) WritePacket(proto.Packet) error         { return nil }
func (testPlayer) BundleHandler() *BundleDelimiterHandler { return nil }
func (testPlayer) State() *state.Registry                 { return state.Play }
func (testPlayer) Protocol() proto.Protocol               { return version.Minecraft_1_20_3.Protocol }
func (testPlayer) BackendInFlight() proto.PacketWriter    { return nil }
func (testPlayer) Disconnect(component.Component)         {}

func TestModernHandlerIgnoresUntrackedResourcePackResponse(t *testing.T) {
	h := newModernHandler(testPlayer{}, event.Nop)

	handled, err := h.OnResourcePackResponse(&ResponseBundle{
		ID:     uuid.New(),
		Status: packet.SuccessfulResourcePackResponseStatus,
	})
	if err != nil {
		t.Fatalf("OnResourcePackResponse returned error: %v", err)
	}
	if handled {
		t.Fatal("OnResourcePackResponse handled untracked response")
	}
}
