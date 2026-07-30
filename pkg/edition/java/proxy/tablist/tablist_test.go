package tablist

import (
	"bytes"
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
)

type testViewer struct {
	protocol proto.Protocol
	written  proto.Packet
}

func (t *testViewer) WritePacket(p proto.Packet) error {
	t.written = p
	var buf bytes.Buffer
	return p.Encode(&proto.PacketContext{Protocol: t.protocol}, &buf)
}

func (t *testViewer) Protocol() proto.Protocol {
	return t.protocol
}

func (t *testViewer) IdentifiedKey() crypto.IdentifiedKey {
	return nil
}

func TestSendHeaderFooterAllowsNilComponents(t *testing.T) {
	viewer := &testViewer{protocol: version.Minecraft_1_21_11.Protocol}

	err := SendHeaderFooter(viewer, nil, nil)
	if err != nil {
		t.Fatalf("SendHeaderFooter returned error: %v", err)
	}
	if _, ok := viewer.written.(*packet.HeaderAndFooter); !ok {
		t.Fatalf("expected HeaderAndFooter packet, got %T", viewer.written)
	}
}
