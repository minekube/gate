package player

import (
	"strings"

	"golang.org/x/text/language"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
)

// Settings are the client settings the player gave us.
type Settings interface {
	Locale() language.Tag // Locale of the Minecraft client.
	// ViewDistance returns the client's view distance. This does not guarantee the client will see this many
	// chunks, since your servers are responsible for sending the chunks.
	ViewDistance() uint8
	ChatMode() ChatMode   // The chat setting of the client.
	ChatColors() bool     // Whether the client has chat colors disabled.
	SkinParts() SkinParts // The parts of player skin the client will show.
	MainHand() MainHand   // The primary hand of the client.
	// ClientListing returns whether the client explicitly
	// allows listing on the TabList in anonymous tab list mode.
	//
	// This feature was introduced in 1.18.
	ClientListing() bool
	TextFiltering() bool            // Whether the client has text filtering enabled.
	ParticleStatus() ParticleStatus // The particle status of the client.
}

var DefaultSettings = NewSettings(&packet.ClientSettings{
	Locale:               "en_US",
	ViewDistance:         2,
	ChatVisibility:       0,
	ChatColors:           true,
	SkinParts:            0,
	MainHand:             1,
	TextFilteringEnabled: false,
	ClientListingAllowed: false,
	ParticleStatus:       int(AllParticleStatus),
})

type ParticleStatus int

const (
	AllParticleStatus ParticleStatus = iota
	DecreasedParticleStatus
	MinimalParticleStatus
)

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

// ClientListing is supported since 1.18.
func (s *clientSettings) ClientListing() bool { return s.s.ClientListingAllowed }

// SkinParts is supported since 1.8.
func (s *clientSettings) SkinParts() SkinParts {
	return SkinParts(s.s.SkinParts)
}

// MainHand is supported since 1.9.
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
	switch s.s.ChatVisibility {
	case 0:
		return ShownChatMode
	case 1:
		return CommandsOnly
	case 2:
		return Hidden
	default:
		return ShownChatMode
	}
}

func (s *clientSettings) ChatColors() bool {
	return s.s.ChatColors
}

func (s *clientSettings) TextFiltering() bool {
	return s.s.TextFilteringEnabled
}

func (s *clientSettings) ParticleStatus() ParticleStatus {
	return ParticleStatus(s.s.ParticleStatus)
}

func NewSettings(packet *packet.ClientSettings) Settings {
	return &clientSettings{
		s:      packet,
		locale: language.Make(strings.ReplaceAll(packet.Locale, "_", "-")),
	}
}
