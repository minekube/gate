package player

import (
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"golang.org/x/text/language"
	"strings"
)

// Settings are the client settings the player gave us.
type Settings interface {
	Locale() language.Tag // Locale of the Minecraft client.
	// Returns the client's view distance. This does not guarantee the client will see this many
	// chunks, since your servers are responsible for sending the chunks.
	ViewDistance() uint8
	ChatMode() ChatMode   // The chat setting of the client.
	ChatColors() bool     // Whether or not the client has chat colors disabled.
	SkinParts() SkinParts // The parts of player skins the client will show.
	MainHand() MainHand   // The primary hand of the client.
}

var DefaultSettings = NewSettings(&packet.ClientSettings{
	Locale:       "en_US",
	ViewDistance: 10,
	ChatColors:   true,
	SkinParts:    127,
	MainHand:     1,
})

type ChatMode string

const (
	ShownChatMode ChatMode = "shown"
	CommandsOnly  ChatMode = "commandsOnly"
	Hidden        ChatMode = "hidden"
)

type MainHand string

const (
	LeftMainHand  MainHand = "left"
	RightMainHand MainHand = "right"
)

type SkinParts byte

func (bitmask SkinParts) Cape() bool {
	return (bitmask & 1) == 1
}
func (bitmask SkinParts) Jacket() bool {
	return ((bitmask >> 1) & 1) == 1
}
func (bitmask SkinParts) LeftSleeve() bool {
	return ((bitmask >> 2) & 1) == 1
}
func (bitmask SkinParts) RightSleeve() bool {
	return ((bitmask >> 3) & 1) == 1
}
func (bitmask SkinParts) LeftPants() bool {
	return ((bitmask >> 4) & 1) == 1
}
func (bitmask SkinParts) RightPants() bool {
	return ((bitmask >> 5) & 1) == 1
}
func (bitmask SkinParts) Hat() bool {
	return ((bitmask >> 6) & 1) == 1
}

type clientSettings struct {
	locale language.Tag
	s      *packet.ClientSettings
}

func (s *clientSettings) SkinParts() SkinParts {
	return SkinParts(s.s.SkinParts)
}

func (s *clientSettings) MainHand() MainHand {
	if s.s.MainHand == 0 {
		return LeftMainHand
	}
	return RightMainHand
}

func (s *clientSettings) Locale() language.Tag {
	return s.locale
}

func (s *clientSettings) ViewDistance() uint8 {
	return s.s.ViewDistance
}

func (s *clientSettings) ChatMode() ChatMode {
	if s.s.ChatVisibility <= 0 || s.s.ChatVisibility > 2 {
		return ShownChatMode
	}
	if s.s.ChatVisibility == 1 {
		return CommandsOnly
	}
	return Hidden
}

func (s *clientSettings) ChatColors() bool {
	return s.s.ChatColors
}

func NewSettings(packet *packet.ClientSettings) Settings {
	return &clientSettings{s: packet, locale: language.Make(strings.ReplaceAll(packet.Locale, "_", "-"))}
}
