package proxy

import (
	"fmt"
	"strconv"
	"strings"

	"go.minekube.com/brigodier"
	. "go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/command/suggest"
)

const glistCmdPermission = "gate.command.glist"

// command for global server+player list
func newGlistCmd(proxy *Proxy) brigodier.LiteralNodeBuilder {
	const glistServerArg = "server"
	return brigodier.Literal("glist").
		Requires(hasCmdPerm(proxy, glistCmdPermission)).
		Executes(command.Command(func(c *command.Context) error {
			return c.SendMessage(glistTotalCount(proxy.PlayerCount()))
		})).
		Then(brigodier.Argument(glistServerArg, brigodier.String).
			Suggests(command.SuggestFunc(func(_ *command.Context,
				b *brigodier.SuggestionsBuilder) *brigodier.Suggestions {
				return suggest.Similar(b, append(serverNames(proxy), "all")).Build()
			})).
			Executes(command.Command(func(c *command.Context) error {
				return glistSendServerCount(proxy, c.Source, c.String(glistServerArg))
			})),
		)
}

func glistTotalCount(count int) Component {
	const allCmd = "/glist all"
	return &Text{Extra: []Component{
		glistTotalProxyCount(count),
		&Text{
			Content: "\nTo view all players on servers, use ",
			S: Style{
				Color:      Yellow,
				ClickEvent: SuggestCommand(allCmd),
			},
			Extra: []Component{
				&Text{Content: allCmd, S: Style{Color: White}},
				&Text{Content: "."},
			},
		},
	}}
}

func glistTotalProxyCount(count int) Component {
	return &Text{S: Style{Color: Yellow}, Extra: []Component{
		&Text{Content: fmt.Sprintf("There %s ", plural("is", count))},
		&Text{Content: strconv.Itoa(count), S: Style{Color: Green}},
		&Text{Content: fmt.Sprintf(" %s online.", plural("player", count))},
	}}
}

func glistSendServerCount(proxy *Proxy, s command.Source, serverName string) error {
	if strings.EqualFold(serverName, "all") {
		servers := proxy.Servers()
		sortServers(servers)
		for _, server := range servers {
			err := s.SendMessage(glistServerPlayers(server, true))
			if err != nil {
				return err
			}
		}
		return s.SendMessage(glistTotalProxyCount(proxy.PlayerCount()))
	}

	server := proxy.Server(serverName)
	if server == nil {
		return s.SendMessage(&Text{S: Style{Color: Red}, Content: fmt.Sprintf("Server %q doesn't exist.", serverName)})
	}

	return s.SendMessage(glistServerPlayers(server, false))
}

// may return nil if server is irrelevant -> empty && fromAll
func glistServerPlayers(server RegisteredServer, fromAll bool) Component {
	onServer := server.Players()
	if onServer.Len() == 0 && fromAll {
		return nil
	}

	var i int
	b := new(strings.Builder)
	onServer.Range(func(p Player) bool {
		if i != 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Username())
		i++
		return true
	})

	return &Text{Extra: []Component{
		&Text{Content: fmt.Sprintf("[%s] ", server.ServerInfo().Name()), S: Style{Color: Aqua}},
		&Text{Content: fmt.Sprintf("(%d)", onServer.Len()), S: Style{Color: Gray}},
		&Text{Content: ": "},
		&Text{Content: b.String()},
	}}
}
