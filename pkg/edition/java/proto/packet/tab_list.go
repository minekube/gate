package packet

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type HeaderAndFooter struct {
	Header string
	Footer string
}

func (h *HeaderAndFooter) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, h.Header)
	if err != nil {
		return err
	}
	return util.WriteString(wr, h.Footer)
}

// we never read this packet
func (h *HeaderAndFooter) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	h.Header, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	h.Footer, err = util.ReadString(rd)
	return err
}

var ResetHeaderAndFooter = &HeaderAndFooter{
	Header: `{"translate":""}`,
	Footer: `{"translate":""}`,
}

var (
	_ proto.Packet = (*HeaderAndFooter)(nil)
	_ proto.Packet = (*PlayerListItem)(nil)
)

//
//
//
//

type PlayerListItem struct {
	Action    PlayerListItemAction
	Items     []PlayerListItemEntry
	PlayerKey crypto.IdentifiedKey // 1.19+
}

type PlayerListItemAction int

const (
	AddPlayerListItemAction PlayerListItemAction = iota
	UpdateGameModePlayerListItemAction
	UpdateLatencyPlayerListItemAction
	UpdateDisplayNamePlayerListItemAction
	RemovePlayerListItemAction
)

type PlayerListItemEntry struct {
	ID          uuid.UUID
	Name        string
	Properties  []profile.Property
	GameMode    int
	Latency     int
	DisplayName component.Component  // nil-able
	PlayerKey   crypto.IdentifiedKey // nil-able - 1.19
}

func (p *PlayerListItem) Encode(c *proto.PacketContext, wr io.Writer) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		err = util.WriteVarInt(wr, int(p.Action))
		if err != nil {
			return err
		}
		err = util.WriteVarInt(wr, len(p.Items))
		if err != nil {
			return err
		}
		for _, item := range p.Items {
			if item.ID == uuid.Nil {
				return errors.New("UUID-less entry serialization attempt - 1.7 component")
			}
			err = util.WriteUUID(wr, item.ID)
			if err != nil {
				return err
			}
			switch p.Action {
			case AddPlayerListItemAction:
				err = util.WriteString(wr, item.Name)
				if err != nil {
					return err
				}
				err = util.WriteProperties(wr, item.Properties)
				if err != nil {
					return err
				}
				err = util.WriteVarInt(wr, item.GameMode)
				if err != nil {
					return err
				}
				err = util.WriteVarInt(wr, item.Latency)
				if err != nil {
					return err
				}
				err = writeDisplayName(wr, item.DisplayName, c.Protocol)
				if err != nil {
					return err
				}
				if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
					err = util.WriteBool(wr, item.PlayerKey != nil)
					if err != nil {
						return err
					}
					if item.PlayerKey != nil {
						err = util.WritePlayerKey(wr, item.PlayerKey)
						if err != nil {
							return err
						}
					}
				}
			case UpdateLatencyPlayerListItemAction:
				err = util.WriteVarInt(wr, item.Latency)
				if err != nil {
					return err
				}
			case UpdateGameModePlayerListItemAction:
				err = util.WriteVarInt(wr, item.GameMode)
				if err != nil {
					return err
				}
			case UpdateDisplayNamePlayerListItemAction:
				err = writeDisplayName(wr, item.DisplayName, c.Protocol)
				if err != nil {
					return err
				}
			case RemovePlayerListItemAction:
			// Do nothing, all that is needed is the uuid
			default:
				return fmt.Errorf("unknown PlayerListItemAction %d", p.Action)
			}
		}

		return nil
	}

	if len(p.Items) == 0 {
		return errors.New("items must not be empty")
	}
	item := p.Items[0]
	displayName := item.DisplayName
	if displayName != nil {
		legacyDisplayName := new(strings.Builder)
		err = util.DefaultJsonCodec().Marshal(legacyDisplayName, displayName)
		if err != nil {
			return fmt.Errorf("error marshal legacy display name: %w", err)
		}
		err = util.WriteString(wr, func() string {
			if legacyDisplayName.Len() > 16 {
				return legacyDisplayName.String()[:16]
			}
			return legacyDisplayName.String()
		}())
		if err != nil {
			return err
		}
	} else {
		err = util.WriteString(wr, item.Name)
		if err != nil {
			return err
		}
	}
	err = util.WriteBool(wr, p.Action != RemovePlayerListItemAction)
	if err != nil {
		return err
	}
	return util.WriteInt16(wr, int16(item.Latency))
}

func writeDisplayName(wr io.Writer, displayName component.Component, protocol proto.Protocol) (err error) {
	err = util.WriteBool(wr, displayName != nil)
	if err != nil {
		return err
	}
	if displayName != nil {
		b := new(strings.Builder)
		err = util.JsonCodec(protocol).Marshal(b, displayName)
		if err != nil {
			return fmt.Errorf("error marshal display name: %w", err)
		}
		err = util.WriteString(wr, b.String())
	}
	return
}

func (p *PlayerListItem) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		action, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		p.Action = PlayerListItemAction(action)
		length, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		for i := 0; i < length; i++ {
			item := PlayerListItemEntry{}
			item.ID, err = util.ReadUUID(rd)
			if err != nil {
				return err
			}
			switch p.Action {
			case AddPlayerListItemAction:
				item.Name, err = util.ReadString(rd)
				if err != nil {
					return err
				}
				item.Properties, err = util.ReadProperties(rd)
				if err != nil {
					return err
				}
				item.GameMode, err = util.ReadVarInt(rd)
				if err != nil {
					return err
				}
				item.Latency, err = util.ReadVarInt(rd)
				if err != nil {
					return err
				}
				item.DisplayName, err = readOptionalComponent(rd, c.Protocol)
				if err != nil {
					return err
				}
				if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
					ok, err := util.ReadBool(rd)
					if err != nil {
						return err
					}
					if ok {
						p.PlayerKey, err = util.ReadPlayerKey(rd)
						if err != nil {
							return err
						}
					}
				}
			case UpdateGameModePlayerListItemAction:
				item.GameMode, err = util.ReadVarInt(rd)
				if err != nil {
					return err
				}
			case UpdateLatencyPlayerListItemAction:
				item.Latency, err = util.ReadVarInt(rd)
				if err != nil {
					return err
				}
			case UpdateDisplayNamePlayerListItemAction:
				item.DisplayName, err = readOptionalComponent(rd, c.Protocol)
				if err != nil {
					return err
				}
			case RemovePlayerListItemAction:
			// Do nothing, all that is needed is the uuid
			default:
				return fmt.Errorf("unknown PlayerListItemAction %d", p.Action)
			}
			p.Items = append(p.Items, item)
		}

		return nil
	}

	var item PlayerListItemEntry
	item.Name, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	actionBool, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	if actionBool {
		p.Action = AddPlayerListItemAction
	} else {
		p.Action = RemovePlayerListItemAction
	}
	latency, err := util.ReadInt16(rd)
	if err != nil {
		return err
	}
	item.Latency = int(latency)
	p.Items = append(p.Items, item)
	return nil
}

func readOptionalComponent(rd io.Reader, protocol proto.Protocol) (c component.Component, err error) {
	var has bool
	has, err = util.ReadBool(rd)
	if !has {
		return
	}
	s, err := util.ReadString(rd)
	if err != nil {
		return nil, err
	}
	return util.JsonCodec(protocol).Unmarshal([]byte(s))
}
