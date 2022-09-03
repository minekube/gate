// Simple example embedding and extending Gate.
package main

import (
	"context"
	"fmt"
	"time"

	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/cmd/gate"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/bossbar"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/event"
)

func main() {
	// Add our "plug-in" to be initialized on Gate start.
	proxy.Plugins = append(proxy.Plugins, proxy.Plugin{
		Name: "SimpleProxy",
		Init: func(proxy *proxy.Proxy) error {
			return newSimpleProxy(proxy).init()
		},
	})

	// Execute Gate entrypoint and block until shutdown.
	// We could also run gate.Start if we don't need Gate's command-line.
	gate.Execute()
}

// SimpleProxy is a simple proxy to showcase some features of Gate.
//
// In this example:
//   - Add a `/broadcast` command
//   - Send a message when player switches the server
//   - Show boss bars to players
type SimpleProxy struct {
	*proxy.Proxy
	legacyCodec *legacy.Legacy
}

func newSimpleProxy(proxy *proxy.Proxy) *SimpleProxy {
	return &SimpleProxy{
		Proxy:       proxy,
		legacyCodec: &legacy.Legacy{Char: legacy.AmpersandChar},
	}
}

// initialize our sample proxy
func (p *SimpleProxy) init() error {
	p.registerCommands()
	return p.registerSubscribers()
}

// Register a proxy-wide commands (can be run while being on any server)
func (p *SimpleProxy) registerCommands() {
	// Registers the "/broadcast" command
	p.Command().Register(brigodier.Literal("broadcast").Then(
		// Adds message argument as in "/broadcast <message>"
		brigodier.Argument("message", brigodier.StringPhrase).
			// Adds completion suggestions as in "/broadcast [suggestions]"
			Suggests(command.SuggestFunc(func(
				c *command.Context,
				b *brigodier.SuggestionsBuilder,
			) *brigodier.Suggestions {
				player, ok := c.Source.(proxy.Player)
				if ok {
					b.Suggest("&oI am &6&l" + player.Username())
				}
				b.Suggest("Hello world!")
				return b.Build()
			})).
			// Executed when running "/broadcast <message>"
			Executes(command.Command(func(c *command.Context) error {
				// Colorize/format message
				message, err := p.legacyCodec.Unmarshal([]byte(c.String("message")))
				if err != nil {
					return c.Source.SendMessage(&Text{
						Content: fmt.Sprintf("Error formatting message: %v", err)})
				}

				// Send to all players on this proxy
				for _, player := range p.Players() {
					// Send message in new goroutine,
					// to not halt loop on slow connections.
					go func(p proxy.Player) { _ = p.SendMessage(message) }(player)
				}
				return nil
			})),
	))
}

// Register event subscribers
func (p *SimpleProxy) registerSubscribers() error {
	// Send message on server switch.
	event.Subscribe(p.Event(), 0, p.onServerSwitch)

	// Change the MOTD response.
	event.Subscribe(p.Event(), 0, pingHandler())

	// Show a boss bar to all players on this proxy.
	event.Subscribe(p.Event(), 0, p.bossBarDisplay())

	return nil
}

func (p *SimpleProxy) onServerSwitch(e *proxy.ServerPostConnectEvent) {
	newServer := e.Player().CurrentServer()
	if newServer == nil {
		return
	}

	_ = e.Player().SendMessage(&Text{
		S: Style{Color: color.Aqua},
		Extra: []Component{
			&Text{
				Content: "\nWelcome to the Gate Sample proxy!\n\n",
				S:       Style{Color: color.Green, Bold: True},
			},
			&Text{Content: "You connected to "},
			&Text{Content: newServer.Server().ServerInfo().Name(), S: Style{Color: color.Yellow}},
			&Text{Content: "."},
			&Text{
				S: Style{
					ClickEvent: SuggestCommand("/broadcast Gate is awesome!"),
					HoverEvent: ShowText(&Text{Content: "/broadcast Gate is awesome!"}),
				},
				Content: "\n\nClick me to run ",
				Extra: []Component{&Text{
					Content: "/broadcast Gate is awesome!",
					S:       Style{Color: color.White, Bold: True, Italic: True},
				}},
			},
		},
	})
}

func pingHandler() func(p *proxy.PingEvent) {
	motd := &Text{Content: "Simple Proxy!\nJoin and test me."}
	return func(e *proxy.PingEvent) {
		p := e.Ping()
		p.Description = motd
		p.Players.Max = p.Players.Online + 1
	}
}

func (p *SimpleProxy) bossBarDisplay() func(*proxy.LoginEvent) {
	// Create shared boss bar for all players
	sharedBar := bossbar.New(
		&Text{Content: "Welcome to Gate Sample proxy!", S: Style{
			Color: color.Aqua,
			Bold:  True,
		}},
		1,
		bossbar.BlueColor,
		bossbar.ProgressOverlay,
	)

	updateBossBar := func(bar bossbar.BossBar, player proxy.Player) {
		now := time.Now()
		text := &Text{Extra: []Component{
			&Text{
				Content: fmt.Sprintf("Hello %s! ", player.Username()),
				S:       Style{Color: color.Yellow},
			},
			&Text{
				Content: fmt.Sprintf("It's %s", now.Format("15:04:05 PM")),
				S:       Style{Color: color.Gold},
			},
		}}
		bar.SetName(text)
		bar.SetPercent(float32(now.Second()) / 60)
	}

	return func(e *proxy.LoginEvent) {
		if !e.Allowed() {
			return
		}
		player := e.Player()

		// Add player to shared boss bar
		_ = player.ShowBossBar(sharedBar)

		// Create own boss bar for player
		playerBar := bossbar.New(
			&Text{},
			bossbar.MinProgress,
			bossbar.RedColor,
			bossbar.ProgressOverlay,
		)
		// Show it to player
		_ = player.ShowBossBar(playerBar)

		// Update boss bars every second,
		// run in new goroutine to not unblock login event handler.
		go func() {
			// Don't forget to remove the boss bar from manager
			// when player disconnects
			defer p.BossBarManager().Unregister(playerBar)
			// Blocking until player disconnects
			tick(player.Context(), time.Second, func() {
				updateBossBar(playerBar, player)
			})
		}()
	}
}

// tick runs a function every interval until the context is cancelled.
func tick(ctx context.Context, interval time.Duration, fn func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fn()
		case <-ctx.Done():
			return
		}
	}
}
