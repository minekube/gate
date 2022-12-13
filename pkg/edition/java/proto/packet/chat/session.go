package chat

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// RemoteChatSession represents a remote chat session that is used to send messages.
type RemoteChatSession struct {
	ID  uuid.UUID // may be zero
	Key crypto.IdentifiedKey
}

func (r *RemoteChatSession) SessionID() uuid.UUID {
	//TODO implement me
	panic("implement me")
}

func (r *RemoteChatSession) IdentifiedKey() crypto.IdentifiedKey {
	//TODO implement me
	panic("implement me")
}

func (r *RemoteChatSession) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteUUID(wr, r.ID)
	if err != nil {
		return err
	}
	return crypto.WritePlayerKey(wr, r.Key)
}

func (r *RemoteChatSession) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r.ID, err = util.ReadUUID(rd)
	if err != nil {
		return err
	}
	r.Key, err = crypto.ReadPlayerKey(c.Protocol, rd)
	return err
}

var _ proto.Packet = (*RemoteChatSession)(nil)
