package packet

import (
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/util"
	"io"
)

type (
	StatusRequest  struct{}
	StatusResponse struct {
		Status string
	}
	StatusPing struct {
		RandomId int64
	}
	// TODO LegacyPing
)

func (s *StatusPing) Encode(_ *proto.PacketContext, wr io.Writer) error {
	return util.WriteInt64(wr, s.RandomId)
}

func (s *StatusPing) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	s.RandomId, err = util.ReadInt64(rd)
	return
}

func (s *StatusResponse) Encode(_ *proto.PacketContext, wr io.Writer) error {
	return util.WriteString(wr, s.Status)
}
func (s *StatusResponse) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	s.Status, err = util.ReadString(rd)
	return
}

func (StatusRequest) Encode(_ *proto.PacketContext, _ io.Writer) error {
	return nil // has no data
}
func (StatusRequest) Decode(_ *proto.PacketContext, _ io.Reader) error {
	return nil // has no data
}

var (
	_ proto.Packet = (*StatusRequest)(nil)
	_ proto.Packet = (*StatusResponse)(nil)
	_ proto.Packet = (*StatusPing)(nil)
)
