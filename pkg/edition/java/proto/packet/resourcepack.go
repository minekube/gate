package packet

import (
	"errors"
	"go.minekube.com/gate/pkg/util/uuid"
	"io"
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type ResourcePackRequest struct {
	ID       uuid.UUID // 1.20.3+
	URL      string
	Hash     string
	Required bool                // 1.17+
	Prompt   component.Component // (nil-able) 1.17+
}

func (r *ResourcePackRequest) Encode(c *proto.PacketContext, wr io.Writer) error {
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_3) {
		if r.ID == uuid.Nil {
			return errors.New("resource pack id is missing")
		}
		err := util.WriteUUID(wr, r.ID)
		if err != nil {
			return err
		}
	}

	if len(r.URL) == 0 {
		return errors.New("url is missing")
	}
	err := util.WriteString(wr, r.URL)
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
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_3) {
		r.ID, err = util.ReadUUID(rd)
		if err != nil {
			return err
		}
	}

	r.URL, err = util.ReadString(rd)
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

type (
	ResourcePackResponse struct {
		ID     uuid.UUID // 1.20.3+
		Hash   string
		Status ResourcePackResponseStatus
	}
	ResourcePackResponseStatus int
)

const (
	SuccessfulResourcePackResponseStatus ResourcePackResponseStatus = iota
	DeclinedResourcePackResponseStatus
	FailedDownloadResourcePackResponseStatus
	AcceptedResourcePackResponseStatus
)

func (r *ResourcePackResponse) Encode(c *proto.PacketContext, wr io.Writer) error {
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_3) {
		err := util.WriteUUID(wr, r.ID)
		if err != nil {
			return err
		}
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_9_4) {
		err := util.WriteString(wr, r.Hash)
		if err != nil {
			return err
		}
	}
	return util.WriteVarInt(wr, int(r.Status))
}

func (r *ResourcePackResponse) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_3) {
		r.ID, err = util.ReadUUID(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_9_4) {
		r.Hash, err = util.ReadString(rd)
		if err != nil {
			return err
		}
	}
	status, err := util.ReadVarInt(rd)
	r.Status = ResourcePackResponseStatus(status)
	return
}

var _ proto.Packet = (*ResourcePackResponse)(nil)

type RemoveResourcePack struct {
	ID uuid.UUID
}

var _ proto.Packet = (*RemoveResourcePack)(nil)

func (r *RemoveResourcePack) Decode(c *proto.PacketContext, rd io.Reader) error {
	hasID, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	if hasID {
		r.ID, err = util.ReadUUID(rd)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RemoveResourcePack) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteBool(wr, r.ID != uuid.Nil)
	if err != nil {
		return err
	}
	if r.ID != uuid.Nil {
		err = util.WriteUUID(wr, r.ID)
		if err != nil {
			return err
		}
	}
	return nil
}
