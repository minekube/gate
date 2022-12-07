package plugin

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

const (
	BrandChannelLegacy      string = "MC|Brand"
	BrandChannel            string = "minecraft:brand"
	RegisterChannelLegacy   string = "REGISTER"
	RegisterChannel         string = "minecraft:register"
	UnregisterChannelLegacy string = "UNREGISTER"
	UnregisterChannel       string = "minecraft:unregister"
)

var InvalidIdentifierRegex = regexp.MustCompile(`[^a-z0-9\\-_]*`)

// McBrand determines whether this is a brand plugin message.
// This is shown on the client.
func McBrand(p *Message) bool {
	return p != nil &&
		(strings.EqualFold(p.Channel, BrandChannelLegacy) ||
			strings.EqualFold(p.Channel, BrandChannel))
}

// IsRegister determines whether this plugin
// message is being used to register plugin channels.
func IsRegister(p *Message) bool {
	return p != nil &&
		(strings.EqualFold(p.Channel, RegisterChannelLegacy) ||
			strings.EqualFold(p.Channel, RegisterChannel))
}

// IsUnregister determines whether this plugin
// message is being used to unregister plugin channels.
func IsUnregister(p *Message) bool {
	return p != nil &&
		(strings.EqualFold(p.Channel, UnregisterChannelLegacy) ||
			strings.EqualFold(p.Channel, UnregisterChannel))
}

// LegacyRegister determines whether this plugin message is a legacy (<1.13) registration plugin message.
func LegacyRegister(p *Message) bool {
	return p != nil && strings.EqualFold(p.Channel, RegisterChannelLegacy)
}

// LegacyUnregister determines whether this plugin message is a legacy (<1.13) un-registration plugin message.
func LegacyUnregister(p *Message) bool {
	return p != nil && strings.EqualFold(p.Channel, UnregisterChannelLegacy)
}

// Channels fetches all the channels in a register or unregister plugin message.
func Channels(p *Message) (channels []string) {
	if p == nil || len(p.Data) == 0 || (!IsRegister(p) && !IsUnregister(p)) {
		return
	}
	return strings.Split(string(p.Data), "\000") // split null-terminated
}

// TransformLegacyToModernChannel transforms a plugin
// message channel from a "legacy" (<1.13) form to a modern one.
func TransformLegacyToModernChannel(name string) string {
	if strings.Contains(name, ":") {
		// Probably valid. We won't check this for now and go on faith.
		return name
	}

	// Before falling into the fallback, explicitly rewrite certain messages.
	switch name {
	case RegisterChannelLegacy:
		return RegisterChannel
	case UnregisterChannelLegacy:
		return UnregisterChannel
	case BrandChannelLegacy:
		return BrandChannel
	case "BungeeCord":
		// This is a special historical case we are compelled to support.
		return "bungeecord:main"
	default:
		// This is very likely a legacy name, so transform it. This proxy uses the same scheme as
		// BungeeCord does to transform channels, but also removes clearly invalid characters as
		// well.
		lower := strings.ToLower(name)
		return "legacy:" + InvalidIdentifierRegex.ReplaceAllString(lower, "")
	}
}

// ConstructChannelsPacket constructs a channel (un)register packet.
// channels must not be empty! Note that the Message's Retained field remains nil.
func ConstructChannelsPacket(protocol proto.Protocol, channels ...string) *Message {
	if len(channels) == 0 {
		panic("channels must not be empty")
	}
	var channelName string
	if protocol.GreaterEqual(version.Minecraft_1_13) {
		channelName = RegisterChannel
	} else {
		channelName = RegisterChannelLegacy
	}
	data := strings.Join(channels, "\000")
	return &Message{
		Channel: channelName,
		Data:    []byte(data),
	}
}

// RewriteMinecraftBrand rewrites the brand message to indicate the presence of the proxy.
func RewriteMinecraftBrand(message *Message, protocol proto.Protocol) *Message {
	if message == nil || !McBrand(message) {
		return message
	}

	currentBrand := readBrandMessage(message.Data)
	rewrittenBrand := fmt.Sprintf("%s (Gate by Minekube)", currentBrand)

	rewrittenBuf := new(bytes.Buffer)
	if protocol.GreaterEqual(version.Minecraft_1_8) {
		_ = util.WriteString(rewrittenBuf, rewrittenBrand)
	} else {
		rewrittenBuf.WriteString(rewrittenBrand)
	}

	return &Message{
		Channel: message.Channel,
		Data:    rewrittenBuf.Bytes(),
	}
}

// Some clients (mostly poorly-implemented bots) do not send validly-formed brand messages.
// In order to accommodate their broken behavior, we'll first try to read in the 1.8 format, and
// if that fails, treat it as a 1.7-format message (which has no prefixed length).
// (The message the proxy sends will be in the correct format depending on the protocol.)
func readBrandMessage(data []byte) string {
	s, err := util.ReadString(bytes.NewReader(data))
	if err != nil {
		s, _ = util.ReadStringWithoutLen(bytes.NewReader(data))
	}
	return s
}
