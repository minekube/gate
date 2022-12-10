package playerinfo

import (
	"io"
	"math"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type (
	Upsert struct {
		ActionSet []UpsertAction
		Entries   []*Entry
	}
	Entry struct {
		ProfileID         uuid.UUID
		Profile           *profile.GameProfile
		Listed            bool
		Latency           int // in milliseconds
		GameMode          int
		DisplayName       component.Component     // nil-able
		RemoteChatSession *chat.RemoteChatSession // nil-able
	}
)

type bitSet []byte

func (b bitSet) set(i int, v bool) {
	if v {
		b[i/8] |= 1 << uint(i%8)
	} else {
		b[i/8] &^= 1 << uint(i%8)
	}
}

func (u *Upsert) Encode(c *proto.PacketContext, wr io.Writer) error {
	set := bitSet(make([]byte, -floorDiv(-len(UpsertActions), 8)))
	for i := range UpsertActions {
		set.set(i, ContainsAction(u.ActionSet, UpsertActions[i]))
	}
	if _, err := wr.Write(set); err != nil {
		return err
	}
	if err := util.WriteVarInt(wr, len(u.Entries)); err != nil {
		return err
	}
	for _, entry := range u.Entries {
		if err := util.WriteUUID(wr, entry.ProfileID); err != nil {
			return err
		}
		for _, action := range u.ActionSet {
			if err := action.Encode(c, wr, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

// ContainsAction returns true if the given action is contained in the given action set.
func ContainsAction(actions []UpsertAction, action UpsertAction) bool {
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}

func (u *Upsert) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	bytes := make([]byte, -floorDiv(-len(UpsertActions), 8))
	if _, err = io.ReadFull(rd, bytes); err != nil {
		return err
	}

	u.ActionSet = nil
	for i, action := range UpsertActions {
		if bytes[i/8]&(1<<uint(i%8)) != 0 {
			u.ActionSet = append(u.ActionSet, action)
		}
	}

	length, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	for i := 0; i < length; i++ {
		entry := new(Entry)
		if entry.ProfileID, err = util.ReadUUID(rd); err != nil {
			return err
		}
		for _, action := range u.ActionSet {
			if err = action.Decode(c, rd, entry); err != nil {
				return err
			}
		}
		u.Entries = append(u.Entries, entry)
	}
	return nil
}

func floorDiv(a, b int) int {
	return int(math.Floor(float64(a) / float64(b)))
}

var _ proto.Packet = (*Upsert)(nil)

// UpsertActions
var (
	AddPlayerAction         UpsertAction = &addAction{}
	InitializeChatAction    UpsertAction = &initChatAction{}
	UpdateGameModeAction    UpsertAction = &updateGameModeAction{}
	UpdateListedAction      UpsertAction = &updateListedAction{}
	UpdateLatencyAction     UpsertAction = &updateLatencyAction{}
	UpdateDisplayNameAction UpsertAction = &updateDisplayNameAction{}

	UpsertActions = []UpsertAction{
		AddPlayerAction,
		InitializeChatAction,
		UpdateGameModeAction,
		UpdateListedAction,
		UpdateLatencyAction,
		UpdateDisplayNameAction,
	}
)

type UpsertAction interface {
	Encode(c *proto.PacketContext, wr io.Writer, info *Entry) error
	Decode(c *proto.PacketContext, rd io.Reader, info *Entry) (err error)
}

type addAction struct{}

func (a *addAction) Encode(c *proto.PacketContext, wr io.Writer, info *Entry) error {
	err := util.WriteString(wr, info.Profile.Name)
	if err != nil {
		return err
	}
	return util.WriteProperties(wr, info.Profile.Properties)
}

func (a *addAction) Decode(c *proto.PacketContext, rd io.Reader, info *Entry) (err error) {
	const maxUsernameLength = 16
	name, err := util.ReadStringMax(rd, maxUsernameLength)
	if err != nil {
		return err
	}
	props, err := util.ReadProperties(rd)
	if err != nil {
		return err
	}
	info.Profile = &profile.GameProfile{
		ID:         info.ProfileID,
		Name:       name,
		Properties: props,
	}
	return nil
}

type initChatAction struct{}

func (a *initChatAction) Encode(c *proto.PacketContext, wr io.Writer, info *Entry) error {
	err := util.WriteBool(wr, info.RemoteChatSession != nil)
	if err != nil {
		return err
	}
	if info.RemoteChatSession != nil {
		return info.RemoteChatSession.Encode(c, wr)
	}
	return nil
}

func (a *initChatAction) Decode(c *proto.PacketContext, rd io.Reader, info *Entry) (err error) {
	ok, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	if ok {
		session := new(chat.RemoteChatSession)
		if err = session.Decode(c, rd); err != nil {
			return err
		}
		info.RemoteChatSession = session
	} else {
		info.RemoteChatSession = nil
	}
	return nil
}

type updateGameModeAction struct{}

func (a *updateGameModeAction) Encode(c *proto.PacketContext, wr io.Writer, info *Entry) error {
	return util.WriteVarInt(wr, info.GameMode)
}

func (a *updateGameModeAction) Decode(c *proto.PacketContext, rd io.Reader, info *Entry) (err error) {
	info.GameMode, err = util.ReadVarInt(rd)
	return err
}

type updateListedAction struct{}

func (a *updateListedAction) Encode(c *proto.PacketContext, wr io.Writer, info *Entry) error {
	return util.WriteBool(wr, info.Listed)
}

func (a *updateListedAction) Decode(c *proto.PacketContext, rd io.Reader, info *Entry) (err error) {
	info.Listed, err = util.ReadBool(rd)
	return err
}

type updateLatencyAction struct{}

func (a *updateLatencyAction) Encode(c *proto.PacketContext, wr io.Writer, info *Entry) error {
	return util.WriteVarInt(wr, info.Latency)
}

func (a *updateLatencyAction) Decode(c *proto.PacketContext, rd io.Reader, info *Entry) (err error) {
	info.Latency, err = util.ReadVarInt(rd)
	return err
}

type updateDisplayNameAction struct{}

func (a *updateDisplayNameAction) Encode(c *proto.PacketContext, wr io.Writer, info *Entry) error {
	err := util.WriteBool(wr, info.DisplayName != nil)
	if err != nil {
		return err
	}
	if info.DisplayName != nil {
		return util.WriteComponent(wr, c.Protocol, info.DisplayName)
	}
	return nil
}

func (a *updateDisplayNameAction) Decode(c *proto.PacketContext, rd io.Reader, info *Entry) (err error) {
	ok, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	if ok {
		info.DisplayName, err = util.ReadComponent(rd, c.Protocol)
	} else {
		info.DisplayName = nil
	}
	return err
}
