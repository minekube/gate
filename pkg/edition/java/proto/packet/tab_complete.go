package packet

import (
	"errors"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

const VanillaMaxTabCompleteLen = 2048

type TabCompleteRequest struct {
	Command       string
	TransactionID int
	AssumeCommand bool
	HasPosition   bool
	Position      int64
}

func (t *TabCompleteRequest) Encode(c *proto.PacketContext, wr io.Writer) error {
	if t.Command == "" {
		return errors.New("command is not specified")
	}

	if c.Protocol.GreaterEqual(version.Minecraft_1_13) {
		err := util.WriteVarInt(wr, t.TransactionID)
		if err != nil {
			return err
		}
		return util.WriteString(wr, t.Command)
	}

	err := util.WriteString(wr, t.Command)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_9) {
		err = util.WriteBool(wr, t.AssumeCommand)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		err = util.WriteBool(wr, t.HasPosition)
		if err != nil {
			return err
		}
		if t.HasPosition {
			return util.WriteInt64(wr, t.Position)
		}
	}
	return nil
}

func (t *TabCompleteRequest) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_13) {
		t.TransactionID, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		t.Command, err = util.ReadStringMax(rd, VanillaMaxTabCompleteLen)
		return
	}

	t.Command, err = util.ReadStringMax(rd, VanillaMaxTabCompleteLen)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_9) {
		t.AssumeCommand, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		t.HasPosition, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
		if t.HasPosition {
			t.Position, err = util.ReadInt64(rd)
		}
	}
	return
}

var _ proto.Packet = (*TabCompleteRequest)(nil)

//
//
//
//

var _ proto.Packet = (*TabCompleteResponse)(nil)

type TabCompleteResponse struct {
	TransactionID int
	Start         int
	Length        int
	Offers        []TabCompleteOffer
}

type TabCompleteOffer struct {
	Text    string
	Tooltip *chat.ComponentHolder // nil-able
}

func (t *TabCompleteResponse) Encode(c *proto.PacketContext, wr io.Writer) error {
	if c.Protocol.GreaterEqual(version.Minecraft_1_13) {
		err := util.WriteVarInt(wr, t.TransactionID)
		if err != nil {
			return err
		}
		err = util.WriteVarInt(wr, t.Start)
		if err != nil {
			return err
		}
		err = util.WriteVarInt(wr, t.Length)
		if err != nil {
			return err
		}
		err = util.WriteVarInt(wr, len(t.Offers))
		if err != nil {
			return err
		}
		for _, offer := range t.Offers {
			err = util.WriteString(wr, offer.Text)
			if err != nil {
				return err
			}
			err = util.WriteBool(wr, offer.Tooltip != nil)
			if err != nil {
				return err
			}
			if offer.Tooltip != nil {
				err = offer.Tooltip.Write(wr, c.Protocol)
				if err != nil {
					return err
				}
			}
		}
		return nil
	} else {
		err := util.WriteVarInt(wr, len(t.Offers))
		if err != nil {
			return err
		}
		for _, offer := range t.Offers {
			err = util.WriteString(wr, offer.Text)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func (t *TabCompleteResponse) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_13) {
		t.TransactionID, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		t.Start, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		t.Length, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		var offers int
		offers, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		var (
			offer      string
			hasTooltip bool
			tooltip    *chat.ComponentHolder
		)
		for i := 0; i < offers; i++ {
			offer, err = util.ReadString(rd)
			if err != nil {
				return err
			}
			hasTooltip, err = util.ReadBool(rd)
			if err != nil {
				return err
			}
			if hasTooltip {
				tooltip, err = chat.ReadComponentHolder(rd, c.Protocol)
				if err != nil {
					return err
				}
			}
			t.Offers = append(t.Offers, TabCompleteOffer{
				Text:    offer,
				Tooltip: tooltip,
			})
		}
	} else {
		var offers int
		offers, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		var offer string
		for i := 0; i < offers; i++ {
			offer, err = util.ReadString(rd)
			if err != nil {
				return err
			}
			t.Offers = append(t.Offers, TabCompleteOffer{Text: offer})
		}
	}
	return nil
}
