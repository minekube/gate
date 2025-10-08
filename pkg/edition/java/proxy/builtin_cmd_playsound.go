package proxy

import (
	"fmt"
	"strconv"

	"go.minekube.com/brigodier"
	. "go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/sound"
)

const playSoundCmdPermission = "gate.command.playsound"

// newPlaySoundCmd creates a command for testing the sound API
func newPlaySoundCmd(proxy *Proxy) brigodier.LiteralNodeBuilder {
	const (
		soundNameArg = "sound"
		sourceArg    = "source"
		volumeArg    = "volume"
		pitchArg     = "pitch"
	)

	return brigodier.Literal("playsound").
		Requires(hasCmdPerm(proxy, playSoundCmdPermission)).
		// Play sound with just name (defaults to master source, volume 1.0, pitch 1.0)
		Then(brigodier.Argument(soundNameArg, brigodier.String).
			Executes(command.Command(func(c *command.Context) error {
				player, ok := c.Source.(Player)
				if !ok {
					return c.Source.SendMessage(&Text{S: Style{Color: Red},
						Content: "Only players can play sounds!"})
				}
				soundName := c.String(soundNameArg)
				return playSound(c, player, soundName, packet.SoundSourceMaster, 1.0, 1.0)
			})).
			// Play sound with source
			Then(brigodier.Argument(sourceArg, brigodier.String).
				Executes(command.Command(func(c *command.Context) error {
					player, ok := c.Source.(Player)
					if !ok {
						return c.Source.SendMessage(&Text{S: Style{Color: Red},
							Content: "Only players can play sounds!"})
					}
					soundName := c.String(soundNameArg)
					source, err := sound.ParseSource(c.String(sourceArg))
					if err != nil {
						return c.Source.SendMessage(&Text{S: Style{Color: Red},
							Content: err.Error()})
					}
					return playSound(c, player, soundName, source, 1.0, 1.0)
				})).
				// Play sound with source and volume
				Then(brigodier.Argument(volumeArg, brigodier.String).
					Executes(command.Command(func(c *command.Context) error {
						player, ok := c.Source.(Player)
						if !ok {
							return c.Source.SendMessage(&Text{S: Style{Color: Red},
								Content: "Only players can play sounds!"})
						}
						soundName := c.String(soundNameArg)
						source, err := sound.ParseSource(c.String(sourceArg))
						if err != nil {
							return c.Source.SendMessage(&Text{S: Style{Color: Red},
								Content: err.Error()})
						}
						volume, err := parseFloat32(c.String(volumeArg))
						if err != nil {
							return c.Source.SendMessage(&Text{S: Style{Color: Red},
								Content: fmt.Sprintf("Invalid volume: %v", err)})
						}
						return playSound(c, player, soundName, source, volume, 1.0)
					})).
					// Play sound with source, volume, and pitch
					Then(brigodier.Argument(pitchArg, brigodier.String).
						Executes(command.Command(func(c *command.Context) error {
							player, ok := c.Source.(Player)
							if !ok {
								return c.Source.SendMessage(&Text{S: Style{Color: Red},
									Content: "Only players can play sounds!"})
							}
							soundName := c.String(soundNameArg)
							source, err := sound.ParseSource(c.String(sourceArg))
							if err != nil {
								return c.Source.SendMessage(&Text{S: Style{Color: Red},
									Content: err.Error()})
							}
							volume, err := parseFloat32(c.String(volumeArg))
							if err != nil {
								return c.Source.SendMessage(&Text{S: Style{Color: Red},
									Content: fmt.Sprintf("Invalid volume: %v", err)})
							}
							pitch, err := parseFloat32(c.String(pitchArg))
							if err != nil {
								return c.Source.SendMessage(&Text{S: Style{Color: Red},
									Content: fmt.Sprintf("Invalid pitch: %v", err)})
							}
							return playSound(c, player, soundName, source, volume, pitch)
						})),
					),
				),
			),
		)
}

func playSound(c *command.Context, player Player, soundName string, source packet.SoundSource, volume, pitch float32) error {
	connPlayer, ok := player.(*connectedPlayer)
	if !ok {
		return c.Source.SendMessage(&Text{S: Style{Color: Red},
			Content: "Invalid player type"})
	}

	snd := sound.NewSound(soundName, source).WithVolume(volume).WithPitch(pitch)
	err := sound.Play(connPlayer, snd, connPlayer)
	if err != nil {
		return c.Source.SendMessage(&Text{S: Style{Color: Red},
			Content: fmt.Sprintf("Failed to play sound: %v", err)})
	}

	return c.Source.SendMessage(&Text{S: Style{Color: Green},
		Content: fmt.Sprintf("Playing sound '%s' from source '%s' (volume: %.2f, pitch: %.2f)",
			soundName, source.String(), volume, pitch)})
}

func parseFloat32(s string) (float32, error) {
	f, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0, err
	}
	return float32(f), nil
}
