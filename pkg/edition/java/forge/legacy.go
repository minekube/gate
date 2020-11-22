package forge

import (
	"bytes"
	"errors"
	"go.minekube.com/gate/pkg/edition/java/modinfo"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"strings"
)

const (
	// Clients attempting to connect to 1.8-1.12.2 Forge servers will have
	// this token appended to the hostname in the initial handshake packet.
	HandshakeHostnameToken = `\0FML\0`
	// The channel for legacy forge handshakes.
	LegacyHandshakeChannel = "FML|HS"
	// The reset packet discriminator.
	ResetDataDiscriminator = -2
	// The acknowledgement packet discriminator.
	AckDiscriminator = -1
	// The Server -> Client Hello discriminator.
	ServerHelloDiscriminator = 0
	// The Client -> Server Hello discriminator.
	ClientHelloDiscriminator = 1
	// The Mod List discriminator.
	ModListDiscriminator = 2
	// The Registry discriminator.
	RegistryDiscriminator = 3
)

// The payload for the reset packet.
var LegacyHandshakeResetData = []byte{ResetDataDiscriminator & 0xff, 0}

// HandshakePacketDiscriminator returns the discriminator from the
// FML|HS packet (the first byte in the data).
func HandshakePacketDiscriminator(message *plugin.Message) (byte, bool) {
	if !strings.EqualFold(message.Channel, LegacyHandshakeChannel) {
		return 0, false
	}
	data := message.Data
	if len(data) >= 1 {
		return data[0], true
	}
	return 0, false
}

// ResetPacket returns a new forge reset packet.
func ResetPacket() *plugin.Message {
	data := make([]byte, len(LegacyHandshakeResetData))
	copy(data, LegacyHandshakeResetData)
	return &plugin.Message{
		Channel: LegacyHandshakeChannel,
		Data:    data,
	}
}

// ReadMods returns the mod list from the mod list packet and parses it.
// May be empty.
func ReadMods(message *plugin.Message) ([]modinfo.Mod, error) {
	if message == nil {
		return nil, errors.New("message must not be nil")
	}
	if !strings.EqualFold(message.Channel, LegacyHandshakeChannel) {
		return nil, errors.New("message is not a FML HS plugin message")
	}
	buf := bytes.NewBuffer(message.Data)
	discriminator, _ := buf.ReadByte()
	if discriminator != ModListDiscriminator {
		return nil, nil
	}
	modCount, err := util.ReadVarInt(buf)
	if err != nil {
		return nil, err
	}
	mods := make([]modinfo.Mod, 0, modCount)
	for i := 0; i < modCount; i++ {
		id, err := util.ReadString(buf)
		if err != nil {
			return nil, err
		}
		version, err := util.ReadString(buf)
		if err != nil {
			return nil, err
		}
		mods = append(mods, modinfo.Mod{
			ID:      id,
			Version: version,
		})
	}
	message.Data = buf.Bytes() // left data bytes
	return mods, nil
}
