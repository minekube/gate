package tablist

import (
	"bytes"
	"fmt"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Viewer is a player connection that can write packets.
type Viewer interface {
	proto.PacketWriter
	Protocol() proto.Protocol
}

// SendHeaderFooter updates the tab list header and footer for a Viewer.
func SendHeaderFooter(viewer Viewer, header, footer component.Component) error {
	b := new(bytes.Buffer)
	p := new(packet.HeaderAndFooter)
	j := util.JsonCodec(viewer.Protocol())

	if err := j.Marshal(b, header); err != nil {
		return fmt.Errorf("error marshal header: %w", err)
	}
	p.Header = b.String()
	b.Reset()

	if err := j.Marshal(b, footer); err != nil {
		return fmt.Errorf("error marshal footer: %w", err)
	}
	p.Footer = b.String()

	return viewer.WritePacket(p)
}

// ClearHeaderFooter clears the tab list header and footer for the viewer.
func ClearHeaderFooter(viewer proto.PacketWriter) error {
	return viewer.WritePacket(packet.ResetHeaderAndFooter)
}
