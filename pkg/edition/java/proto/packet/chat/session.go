package chat

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type RemoteChatSession struct {
	SessionID uuid.UUID // zeroable
	crypto.IdentifiedKey
}

func (r *RemoteChatSession) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteUUID(wr, r.SessionID)
	if err != nil {
		return err
	}
	return crypto.WritePlayerKey(wr, r.IdentifiedKey)
}

func (r *RemoteChatSession) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r.SessionID, err = util.ReadUUID(rd)
	if err != nil {
		return err
	}
	r.IdentifiedKey, err = crypto.ReadPlayerKey(c.Protocol, rd)
	return err
}

var _ proto.Packet = (*RemoteChatSession)(nil)
