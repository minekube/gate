package packet

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

type ChatCompletionAction int

const (
	AddChatCompletionAction ChatCompletionAction = iota
	RemoveChatCompletionAction
	AlterChatCompletionAction
)

type PlayerChatCompletion struct {
	Completions []string
	Action      ChatCompletionAction
}

func (p *PlayerChatCompletion) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteVarInt(wr, int(p.Action))
	if err != nil {
		return err
	}
	return util.WriteStrings(wr, p.Completions)
}

func (p *PlayerChatCompletion) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	action, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	p.Action = ChatCompletionAction(action)
	p.Completions, err = util.ReadStringArray(rd)
	return err
}

var _ proto.Packet = (*PlayerChatCompletion)(nil)
