---
title: 'Sound API - Play and Control Sounds'
description: "Use Gate's Sound API to play and stop sounds for players. Control volume, pitch, and sound sources with the sound package."
---

# Sounds

Gate provides a dedicated `sound` package for playing and controlling sounds for Minecraft players.
This API allows you to create immersive audio experiences in your proxy extensions.

::: info Version Requirements

- **Minimum Minecraft Version:** 1.19.3
- **UI Sound Source:** Requires 1.21.5+

Players on older versions will receive an error if they try to use sound features.
:::

## Package Import

```go
import (
    "go.minekube.com/gate/pkg/edition/java/sound"
)
```

## Quick Start

### Playing Sounds

The simplest way to play a sound:

```go
// Create a sound
snd := sound.NewSound("entity.player.levelup", sound.SourcePlayer)

// Play it for the player (emitted from the player's location)
err := sound.Play(player, snd, player)
```

### Stopping Sounds

```go
// Stop all sounds
err := sound.StopAll(player)

// Stop sounds from a specific source
err := sound.StopSource(player, sound.SourceMusic)

// Stop a specific sound
err := sound.StopSound(player, "entity.cat.ambient")
```

## Sound Configuration

The `Sound` struct allows you to customize playback:

```go
snd := sound.NewSound("block.note_block.pling", sound.SourceBlock).
    WithVolume(2.0).  // Louder (also increases range)
    WithPitch(1.5).   // Higher pitch
    WithSeed(12345)   // Specific random seed
```

### Sound Properties

- **Name**: The sound identifier (e.g., `"entity.experience_orb.pickup"`)
- **Source**: The sound category/source (affects volume controls)
- **Volume**: `0.0` to `∞` (values > 1.0 increase audible range)
- **Pitch**: `0.5` to `2.0` (lower = deeper, higher = higher pitch)
- **Seed**: Optional random seed for sound variation

## Sound Sources

Sound sources determine which volume slider in the client controls the sound:

| Description         | Constant                    |
| ------------------- | --------------------------- |
| Master volume       | `sound.SourceMaster`        |
| Background music    | `sound.SourceMusic`         |
| Jukebox/Music discs | `sound.SourceRecord`        |
| Rain, thunder       | `sound.SourceWeather`       |
| Block sounds        | `sound.SourceBlock`         |
| Hostile mobs        | `sound.SourceHostile`       |
| Neutral mobs        | `sound.SourceNeutral`       |
| Player actions      | `sound.SourcePlayer`        |
| Ambient sounds      | `sound.SourceAmbient`       |
| Voice chat          | `sound.SourceVoice`         |
| Interface sounds    | `sound.SourceUI` ⚠️ 1.21.5+ |

### Parsing Sound Sources

```go
// From string
source, err := sound.ParseSource("music")
if err != nil {
    // Invalid source name
}

// To string
name := source.String() // "music"
```

## Playing Sounds from Different Emitters

Sounds can be emitted from different players on the same server:

```go
targetPlayer := ... // player who will hear the sound
emitterPlayer := ... // player from whose location the sound plays

snd := sound.NewSound("entity.villager.yes", sound.SourceNeutral)

// Play sound at emitter's location for target player
err := sound.Play(targetPlayer, snd, emitterPlayer)
```

::: warning Same Server Required
Both the target player and emitter must be connected to the same backend server.
Otherwise, `ErrDifferentServers` will be returned.
:::

## Advanced Stop Options

The `sound.Stop()` function provides flexible filtering:

```go
// Stop specific sound from specific source
source := sound.SourceAmbient
soundName := "entity.cat.ambient"
err := sound.Stop(player, &source, &soundName)

// Stop all sounds from a source (soundName = nil)
err := sound.Stop(player, &source, nil)

// Stop a sound from all sources (source = nil)
err := sound.Stop(player, nil, &soundName)

// Stop everything (both nil)
err := sound.Stop(player, nil, nil)
```

## Complete Example: Event-Based Sounds

Play a sound when a player connects to a server:

```go
<!--@include: ./examples/sound-example-play.go -->
```

## Complete Example: Sound Commands

Create commands to test the sound API:

```go
<!--@include: ./examples/sound-example-command.go -->
```

## Common Sound Names

### UI & Feedback Sounds

- `entity.experience_orb.pickup` - XP orb pickup sound
- `entity.player.levelup` - Level up fanfare
- `ui.button.click` - Button click
- `entity.villager.yes` / `entity.villager.no` - Villager sounds

### Note Block Sounds

- `block.note_block.bell` - Bell
- `block.note_block.pling` - Pling (high pitch)
- `block.note_block.harp` - Harp
- `block.note_block.bass` - Bass
- `block.note_block.guitar` - Guitar

### Entity Sounds

- `entity.enderman.teleport` - Teleport effect
- `entity.arrow.hit_player` - Arrow hit
- `entity.item.pickup` - Item pickup

::: tip Find More Sounds
For a complete list of available sounds, see the [Minecraft Wiki - Sound Events](https://minecraft.wiki/w/Sounds.json).
:::

## Error Handling

The sound API returns specific errors for better handling:

```go
err := sound.Play(player, snd, emitter)
if err != nil {
    switch {
    case errors.Is(err, sound.ErrUnsupportedClientProtocol):
        // Player is on a version < 1.19.3
    case errors.Is(err, sound.ErrUISourceUnsupported):
        // Player tried to use UI source on version < 1.21.5
    case errors.Is(err, sound.ErrNotConnected):
        // Player is not connected to a server
    case errors.Is(err, sound.ErrDifferentServers):
        // Emitter and target are on different servers
    }
}
```

## Known Minecraft Bugs

::: warning Minecraft Limitations

- **MC-146721**: Stereo sounds are always played globally in 1.14+ (not positional)
- **MC-138832**: Volume and pitch are ignored in Minecraft 1.14 to 1.16.5
- Invalid sound names will silently fail (no error, but no sound plays)
  :::

## API Reference

### Functions

#### `sound.Play(player, sound, emitter) error`

Plays a sound at an entity's location.

**Parameters:**

- `player` - The player who will hear the sound
- `sound` - The sound configuration
- `emitter` - The player from whose location the sound plays

**Returns:** Error if version incompatible or players not on same server

#### `sound.Stop(player, source, soundName) error`

Stops sounds based on criteria.

**Parameters:**

- `player` - The player for whom to stop sounds
- `source` - Sound source filter (nil = any)
- `soundName` - Sound name filter (nil = any)

**Returns:** Error if version incompatible

#### Helper Functions

- `sound.StopAll(player) error` - Stop all sounds
- `sound.StopSource(player, source) error` - Stop all sounds from a source
- `sound.StopSound(player, name) error` - Stop a specific sound

## See Also

- [Events Documentation](/developers/events) - Handle player events
- [Commands Documentation](/developers/commands) - Create custom commands
- [Cookie Package](https://pkg.go.dev/go.minekube.com/gate/pkg/edition/java/cookie) - Similar package pattern
- [Boss Bar Package](https://pkg.go.dev/go.minekube.com/gate/pkg/edition/java/bossbar) - Similar package pattern
