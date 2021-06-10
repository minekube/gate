package proxy

import (
	"context"
	"fmt"
	"go.minekube.com/brigodier"
	. "go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/internal/suggest"
	"strings"
	"time"
)

func (p *Proxy) registerBuiltinCommands() {
	p.command.Register(newServerCmd(p))
}

func hasCmdPerm(proxy *Proxy, perm string) brigodier.RequireFn {
	return command.Requires(func(c *command.RequiresContext) bool {
		return !proxy.config.RequireBuiltinCommandPermissions || c.Source.HasPermission(perm)
	})
}

const serverCmdPermission = "gate.command.server"

func newServerCmd(proxy *Proxy) brigodier.LiteralNodeBuilder {
	return brigodier.Literal("server").
		Requires(hasCmdPerm(proxy, serverCmdPermission)).
		// List registered server.
		Executes(command.Command(func(c *command.Context) error {
			const maxEntries = 50
			var servers []Component
			proxyServers := proxy.Servers()
			for i, s := range proxyServers {
				if i+1 == maxEntries {
					servers = append(servers, &Text{
						Content: fmt.Sprintf("and %d more...", len(proxyServers)-i+1),
					})
					break
				}
				servers = append(servers, &Text{
					Content: fmt.Sprintf("  %s - %s (%d players)\n",
						s.ServerInfo().Name(), s.ServerInfo().Addr(), s.Players().Len()),
					S: Style{ClickEvent: RunCommand(fmt.Sprintf("/server %s", s.ServerInfo().Name()))},
				})
			}
			return c.Source.SendMessage(&Text{
				Content: fmt.Sprintf("\nServers (%d):\n", len(proxyServers)),
				S:       Style{Color: Green},
				Extra: []Component{&Text{
					S: Style{
						Color:      Yellow,
						HoverEvent: ShowText(&Text{Content: "Click to connect!", S: Style{Color: Green}}),
					},
					Extra: servers,
				}},
			})
		})).
		// Switch server
		Then(brigodier.Argument("name", brigodier.StringWord).
			Suggests(serverSuggestionProvider(proxy)).
			Executes(command.Command(func(c *command.Context) error {
				player, ok := c.Source.(Player)
				if !ok {
					return c.Source.SendMessage(&Text{Content: "Only players can connect to a server!", S: Style{Color: Red}})
				}

				name := c.String("name")
				rs := proxy.Server(name)
				if rs == nil {
					return c.Source.SendMessage(&Text{Content: fmt.Sprintf("Server %q not registered", name), S: Style{Color: Red}})
				}

				ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(proxy.config.ConnectionTimeout))
				defer cancel()
				player.CreateConnectionRequest(rs).ConnectWithIndication(ctx)
				return nil
			})),
		)
}

func serverSuggestionProvider(p *Proxy) brigodier.SuggestionProvider {
	return suggest.ProviderFunc(func(_ *brigodier.CommandContext, b *brigodier.SuggestionsBuilder) *brigodier.Suggestions {
		servers := p.Servers()
		candidates := make([]string, len(servers))
		for i, s := range servers {
			candidates[i] = s.ServerInfo().Name()
		}
		given := b.Input[strings.Index(b.Input, " ")+1:]
		return suggest.Build(b, given, candidates)
	})
}
