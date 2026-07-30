package main

import (
	"context"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/edition/java/sound"
	"go.minekube.com/gate/pkg/gate"
)

func main() {
	proxy.Plugins = append(proxy.Plugins, proxy.Plugin{
		Name: "SoundExample",
		Init: func(ctx context.Context, p *proxy.Proxy) error {
			return registerSoundExample(p)
		},
	})
	gate.Execute()
}

func registerSoundExample(p *proxy.Proxy) error {
	// Subscribe to server connected event
	p.Event().Subscribe(&proxy.ServerPostConnectEvent{}, 0, func(e *proxy.ServerPostConnectEvent) {
		player := e.Player()

		// Play a level-up sound when player connects to a server
		levelUpSound := sound.NewSound("entity.player.levelup", sound.SourcePlayer).
			WithVolume(1.0).
			WithPitch(1.0)

		_ = sound.Play(player, levelUpSound, player)
	})

	return nil
}
