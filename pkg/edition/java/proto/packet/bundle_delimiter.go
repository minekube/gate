package packet

import (
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type BundleDelimiter struct{}

func (b *BundleDelimiter) Encode(*proto.PacketContext, io.Writer) error {
	return nil
}

func (b *BundleDelimiter) Decode(*proto.PacketContext, io.Reader) error {
	return nil
}

var _ proto.Packet = (*BundleDelimiter)(nil)
