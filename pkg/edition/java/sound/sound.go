// Package sound provides a way to play and stop sounds for Minecraft players.
package sound

import (
	"errors"
	"fmt"
	"math/rand"

	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

var (
	ErrUnsupportedClientProtocol = errors.New("player version must be at least 1.19.3 to use the sound API")
	ErrUISourceUnsupported       = errors.New("UI sound source requires at least 1.21.5")
	ErrNotConnected              = errors.New("player must be connected to a server")
	ErrEmitterNotConnected       = errors.New("emitter player must be connected to a server")
	ErrDifferentServers          = errors.New("emitter player must be on the same server as the target player")
	ErrInvalidEmitter            = errors.New("emitter must be a valid player")
)

const (
	SourceMaster  = packet.SoundSourceMaster
	SourceMusic   = packet.SoundSourceMusic
	SourceRecord  = packet.SoundSourceRecord
	SourceWeather = packet.SoundSourceWeather
	SourceBlock   = packet.SoundSourceBlock
	SourceHostile = packet.SoundSourceHostile
	SourceNeutral = packet.SoundSourceNeutral
	SourcePlayer  = packet.SoundSourcePlayer
	SourceAmbient = packet.SoundSourceAmbient
	SourceVoice   = packet.SoundSourceVoice
	SourceUI      = packet.SoundSourceUI
)

// Player represents a player that can play and stop sounds.
// This is typically a *connectedPlayer from the proxy package.
//
// The Play function uses type assertions to access additional methods
// (CurrentServerEntityID, CheckServerMatch) that are not part of this
// interface, allowing API users to pass proxy.Player without requiring
// those methods in the proxy.Player interface.
type Player interface {
	proto.PacketWriter
	Protocol() proto.Protocol
}

// ParseSource parses a sound source from a string.
func ParseSource(source string) (s packet.SoundSource, err error) {
	err = s.UnmarshalText([]byte(source))
	return
}

// Sound represents a sound that can be played.
type Sound struct {
	Name   key.Key
	Source packet.SoundSource
	Volume float32
	Pitch  float32
	Seed   *int64 // nil for random seed
}

// NewSound creates a new sound with default volume (1.0) and pitch (1.0).
func NewSound(name string, source packet.SoundSource) *Sound {
	return &Sound{
		Name:   key.New(key.MinecraftNamespace, name),
		Source: source,
		Volume: 1.0,
		Pitch:  1.0,
	}
}

// WithVolume sets the volume for the sound.
func (s Sound) WithVolume(volume float32) Sound {
	s.Volume = volume
	return s
}

// WithPitch sets the pitch for the sound.
func (s Sound) WithPitch(pitch float32) Sound {
	s.Pitch = pitch
	return s
}

// WithSeed sets a specific seed for the sound.
func (s Sound) WithSeed(seed int64) Sound {
	s.Seed = &seed
	return s
}

// Play plays a sound at an entity's location.
//
// Note: Due to MC-146721, stereo sounds are always played globally in 1.14+.
// Note: Due to MC-138832, the volume and pitch are ignored in 1.14 to 1.16.5.
//
// This method requires Minecraft 1.19.3+ and requires both the emitter and
// the player to be connected to a server.
//
// emitter can be the player themselves (self) or another player on the same server.
func Play(player Player, sound Sound, emitter Player) error {
	// Check protocol version support
	if player.Protocol().Lower(version.Minecraft_1_19_3) {
		return fmt.Errorf("%w: player is on %s", ErrUnsupportedClientProtocol, player.Protocol())
	}

	// Check for UI sound source on older versions
	if sound.Source == packet.SoundSourceUI && player.Protocol().Lower(version.Minecraft_1_21_5) {
		return fmt.Errorf("%w: player is on %s", ErrUISourceUnsupported, player.Protocol())
	}

	// Type assert to access server entity methods that are not part of the Player interface.
	// This allows API users to pass proxy.Player without requiring those methods in the interface.
	type entityIDProvider interface {
		CurrentServerEntityID() (int, bool)
	}
	type serverMatcher interface {
		CheckServerMatch(other interface{ CurrentServerEntityID() (int, bool) }) bool
	}

	entityProvider, ok := player.(entityIDProvider)
	if !ok {
		return fmt.Errorf("player does not implement required methods for sound playback")
	}

	// Get target player's entity ID
	targetEntityID, ok := entityProvider.CurrentServerEntityID()
	if !ok {
		return ErrNotConnected
	}

	// Determine the emitter's entity ID
	var emitterEntityID int
	if emitter == player {
		// Self emitter
		emitterEntityID = targetEntityID
	} else if emitter != nil {
		// Type assert emitter to entityIDProvider
		emitterProvider, ok := emitter.(entityIDProvider)
		if !ok {
			return ErrInvalidEmitter
		}
		// Check if emitter is on the same server
		matcher, ok := player.(serverMatcher)
		if !ok {
			return fmt.Errorf("player does not implement CheckServerMatch")
		}
		if !matcher.CheckServerMatch(emitterProvider) {
			return ErrDifferentServers
		}
		emitterEntityID, ok = emitterProvider.CurrentServerEntityID()
		if !ok {
			return ErrEmitterNotConnected
		}
	} else {
		return ErrInvalidEmitter
	}

	// Create the sound packet
	soundPacket := &packet.SoundEntityPacket{
		SoundID:     0, // 0 means named sound
		SoundName:   sound.Name,
		SoundSource: sound.Source,
		EntityID:    emitterEntityID,
		Volume:      sound.Volume,
		Pitch:       sound.Pitch,
	}
	if sound.Seed != nil {
		soundPacket.Seed = *sound.Seed
	} else {
		soundPacket.Seed = rand.Int63()
	}

	return player.WritePacket(soundPacket)
}

// Stop stops playing sounds on the player's client.
//
// Either source or soundName (or both) can be nil to stop all sounds matching the criteria.
// - source=nil, soundName=nil: Stop all sounds
// - source=set, soundName=nil: Stop all sounds from the specified source
// - source=nil, soundName=set: Stop the specified sound from all sources
// - source=set, soundName=set: Stop the specified sound from the specified source
//
// This method requires Minecraft 1.19.3+.
func Stop(player Player, source *packet.SoundSource, soundName *string) error {
	// Check protocol version support
	if player.Protocol().Lower(version.Minecraft_1_19_3) {
		return fmt.Errorf("%w: player is on %s", ErrUnsupportedClientProtocol, player.Protocol())
	}

	// Check for UI sound source on older versions
	if source != nil && *source == packet.SoundSourceUI && player.Protocol().Lower(version.Minecraft_1_21_5) {
		return fmt.Errorf("%w: player is on %s", ErrUISourceUnsupported, player.Protocol())
	}

	// Create the sound name key
	var name key.Key
	if soundName != nil && *soundName != "" {
		name = key.New(key.MinecraftNamespace, *soundName)
	}

	// Create the stop sound packet
	stopPacket := &packet.StopSoundPacket{
		Source:    source,
		SoundName: name,
	}

	return player.WritePacket(stopPacket)
}

// StopAll stops all sounds playing on the player's client.
func StopAll(player Player) error {
	return Stop(player, nil, nil)
}

// StopSource stops all sounds from a specific source category.
func StopSource(player Player, source packet.SoundSource) error {
	return Stop(player, &source, nil)
}

// StopSound stops a specific sound from all sources.
func StopSound(player Player, soundName string) error {
	return Stop(player, nil, &soundName)
}
