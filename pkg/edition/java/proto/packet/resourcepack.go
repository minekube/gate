package packet

import (
	"errors"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
	"strings"
)

type ResourcePackRequest struct {
	Url      string
	Hash     string
	Required bool                // 1.17+
	Prompt   component.Component // (nil-able) 1.17+
}

func (r *ResourcePackRequest) Encode(c *proto.PacketContext, wr io.Writer) error {
	if len(r.Url) == 0 {
		return errors.New("url is missing")
	}
	err := util.WriteString(wr, r.Url)
	if err != nil {
		return err
	}
	err = util.WriteString(wr, r.Hash)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_17) {
		err = util.WriteBool(wr, r.Required)
		if err != nil {
			return err
		}
		if r.Prompt != nil {
			err = util.WriteBool(wr, true)
			if err != nil {
				return err
			}
			buf := new(strings.Builder)
			err = util.JsonCodec(c.Protocol).Marshal(buf, r.Prompt)
			if err != nil {
				return err
			}
			err = util.WriteString(wr, buf.String())
		} else {
			err = util.WriteBool(wr, false)
		}
	}
	return err
}

func (r *ResourcePackRequest) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r.Url, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	r.Hash, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_17) {
		r.Required, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
		var hasPrompt bool
		hasPrompt, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
		if hasPrompt {
			var prompt string
			prompt, err = util.ReadString(rd)
			if err != nil {
				return err
			}
			r.Prompt, err = util.JsonCodec(c.Protocol).Unmarshal([]byte(prompt))
		} else {
			r.Prompt = nil
		}
	}
	return err
}

var _ proto.Packet = (*ResourcePackRequest)(nil)
