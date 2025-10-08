package main

import (
	"context"
	"fmt"
	"strconv"

	"go.minekube.com/brigodier"
	. "go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/edition/java/sound"
	"go.minekube.com/gate/pkg/gate"
)

func main() {
	proxy.Plugins = append(proxy.Plugins, proxy.Plugin{
		Name: "SoundCommandExample",
		Init: func(ctx context.Context, p *proxy.Proxy) error {
			return registerSoundCommands(p)
		},
	})
	gate.Execute()
}

func registerSoundCommands(p *proxy.Proxy) error {
	// Register /playsound command
	p.Command().Register(
		brigodier.Literal("playsound").
			Then(brigodier.Argument("sound", brigodier.String).
				Then(brigodier.Argument("source", brigodier.String).
					Then(brigodier.Argument("volume", brigodier.String).
						Then(brigodier.Argument("pitch", brigodier.String).
							Executes(command.Command(func(c *command.Context) error {
								player, ok := c.Source.(proxy.Player)
								if !ok {
									return c.Source.SendMessage(&Text{
										S:       Style{Color: Red},
										Content: "Only players can use this command!",
									})
								}

								soundName := c.String("sound")
								source, err := sound.ParseSource(c.String("source"))
								if err != nil {
									return c.Source.SendMessage(&Text{
										S:       Style{Color: Red},
										Content: err.Error(),
									})
								}

								volume, err := strconv.ParseFloat(c.String("volume"), 32)
								if err != nil {
									return c.Source.SendMessage(&Text{
										S:       Style{Color: Red},
										Content: "Invalid volume",
									})
								}

								pitch, err := strconv.ParseFloat(c.String("pitch"), 32)
								if err != nil {
									return c.Source.SendMessage(&Text{
										S:       Style{Color: Red},
										Content: "Invalid pitch",
									})
								}

								// Create and play the sound
								snd := sound.NewSound(soundName, source).
									WithVolume(float32(volume)).
									WithPitch(float32(pitch))

								if err := sound.Play(player, snd, player); err != nil {
									return c.Source.SendMessage(&Text{
										S:       Style{Color: Red},
										Content: fmt.Sprintf("Error: %v", err),
									})
								}

								return c.Source.SendMessage(&Text{
									S: Style{Color: Green},
									Content: fmt.Sprintf("Playing %s at %.1f volume, %.1f pitch",
										soundName, volume, pitch),
								})
							})),
						),
					),
				),
			),
	)

	// Register /stopsound command
	p.Command().Register(
		brigodier.Literal("stopsound").
			Executes(command.Command(func(c *command.Context) error {
				player, ok := c.Source.(proxy.Player)
				if !ok {
					return c.Source.SendMessage(&Text{
						S:       Style{Color: Red},
						Content: "Only players can use this command!",
					})
				}

				if err := sound.StopAll(player); err != nil {
					return c.Source.SendMessage(&Text{
						S:       Style{Color: Red},
						Content: fmt.Sprintf("Error: %v", err),
					})
				}

				return c.Source.SendMessage(&Text{
					S:       Style{Color: Green},
					Content: "Stopped all sounds",
				})
			})),
	)

	return nil
}
