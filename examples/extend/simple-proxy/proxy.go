// Simple example embedding and extending Gate.
package main

import (
	"fmt"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/cmd/gate"
	"go.minekube.com/gate/pkg/command"
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

// SimpleProxy is a simple proxy that adds a `/broadcast` command
// and sends a message on server switch.
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
	// The message argument as in "/broadcast <message>"
	p.Command().Register(brigodier.Literal("broadcast").Then(
		brigodier.Argument("message", brigodier.StringPhrase).
			// Adds completion suggestions to "/broadcast [suggestions]"
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
	p.Event().Subscribe(&proxy.ServerPostConnectEvent{}, 0, func(ev event.Event) {
		e := ev.(*proxy.ServerPostConnectEvent)

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
	})

	// Change the MOTD response.
	motd := &Text{Content: "Simple Proxy!\nJoin and test me."}
	p.Event().Subscribe(&proxy.PingEvent{}, 0, func(ev event.Event) {
		e := ev.(*proxy.PingEvent)
		p := e.Ping()

		p.Description = motd
		p.Players.Max = p.Players.Online + 1
	})

	return nil
}
