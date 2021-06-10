package proxy

import (
	"context"
	"fmt"
	"go.minekube.com/brigodier"
	. "go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/internal/suggest"
	"sort"
	"time"
)

const serverCmdPermission = "gate.command.server"

// command to list and connect to registered servers
func newServerCmd(proxy *Proxy) brigodier.LiteralNodeBuilder {
	return brigodier.Literal("server").
		Requires(hasCmdPerm(proxy, serverCmdPermission)).
		// List registered server.
		Executes(command.Command(func(c *command.Context) error {
			return c.SendMessage(serversInfo(proxy, c.Source))
		})).
		// Switch server
		Then(brigodier.Argument("name", brigodier.String).
			Suggests(serverSuggestionProvider(proxy)).
			Executes(command.Command(func(c *command.Context) error {
				player, ok := c.Source.(Player)
				if !ok {
					return c.Source.SendMessage(&Text{S: Style{Color: Red},
						Content: "Only players can connect to a server!"})
				}

				name := c.String("name")
				rs := proxy.Server(name)
				if rs == nil {
					return c.Source.SendMessage(&Text{S: Style{Color: Red},
						Content: fmt.Sprintf("Server %q doesn't exist.", name)})
				}

				ctx, cancel := context.WithTimeout(context.Background(),
					time.Millisecond*time.Duration(proxy.config.ConnectionTimeout))
				defer cancel()
				player.CreateConnectionRequest(rs).ConnectWithIndication(ctx)
				return nil
			})),
		)
}

const maxServersToList = 50

func serversInfo(proxy *Proxy, s command.Source) (c Component) {
	info := &Text{Content: "\n", S: Style{Color: Yellow}}
	c = info
	add := func(c Component) { info.Extra = append(info.Extra, c) }

	// Show current server
	var current string
	if p, ok := s.(Player); ok {
		curr := p.CurrentServer()
		if curr != nil {
			current = curr.Server().ServerInfo().Name()
			add(&Text{Content: fmt.Sprintf("You are currently connected to %q.\n", current)})
		}
	}

	servers := proxy.Servers()
	sortServers(servers)

	// Assemble the list of servers as components
	list := &Text{S: Style{Color: Gray}}
	add(&Text{
		Content: fmt.Sprintf("Available servers (%d):\n\n", len(servers)),
		Extra:   []Component{list},
	})
	split := &Text{Content: ", "}
	for i, server := range servers {
		if i+1 == maxServersToList {
			list.Extra = append(list.Extra, &Text{
				Content: fmt.Sprintf(
					"\n\nand %d more servers...", len(servers)-i),
				S: Style{HoverEvent: ShowText(&Text{Content: "Tab-complete to search more servers."})},
			})
			break
		}
		if i != 0 {
			list.Extra = append(list.Extra, split)
		}
		list.Extra = append(list.Extra, formatServerComponent(current, server))
	}
	return
}

func formatServerComponent(currentPlayerServer string, s RegisteredServer) Component {
	name := s.ServerInfo().Name()
	c := &Text{Content: name}
	size := s.Players().Len()
	playersText := fmt.Sprintf("%d %s online", size, plural("player", size))
	cmd := fmt.Sprintf("/server %s", name)
	if currentPlayerServer == name {
		c.S = Style{Color: Red,
			HoverEvent: ShowText(&Text{Content: fmt.Sprintf(
				"Currently connected to this server\n%s", playersText)}),
			ClickEvent: SuggestCommand(cmd),
		}
	} else {
		c.S = Style{Color: Gray,
			HoverEvent: ShowText(&Text{Content: fmt.Sprintf(
				"Click to connect to this server\n%s", playersText)}),
			ClickEvent: RunCommand(cmd),
		}
	}
	return c
}

func plural(s string, i int) string {
	if i == 0 || i > 1 || i < -1 {
		if s == "is" {
			return "are"
		}
		return s + "s"
	}
	return s
}

// sort servers by name
func sortServers(s []RegisteredServer) {
	sort.Slice(s, func(i, j int) bool {
		return s[i].ServerInfo().Name() < s[j].ServerInfo().Name()
	})
}

func serverSuggestionProvider(p *Proxy) brigodier.SuggestionProvider {
	return suggest.ProviderFunc(func(
		_ *brigodier.CommandContext,
		b *brigodier.SuggestionsBuilder,
	) *brigodier.Suggestions {
		return suggest.Build(b, b.Input, serverNames(p))
	})
}

func serverNames(p *Proxy) []string {
	servers := p.Servers()
	n := make([]string, len(servers))
	for i, s := range servers {
		n[i] = s.ServerInfo().Name()
	}
	return n
}
