package proxy

import (
	"fmt"
	"go.minekube.com/brigodier"
	. "go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/command/suggest"
	"strings"
)

const sendCmdPermission = "gate.command.send"

func newSendCmd(proxy *Proxy) brigodier.LiteralNodeBuilder {
	const sendPlayerArg = "player"
	const sendServerArg = "server"
	return brigodier.Literal("send").
		Requires(hasCmdPerm(proxy, sendCmdPermission)).
		Then(brigodier.Argument(sendPlayerArg, brigodier.String).
			Suggests(playerSuggestionProvider(proxy, "all", "current")).
			Then(brigodier.Argument(sendServerArg, brigodier.String).
				Suggests(serverSuggestionProvider(proxy)).
				Executes(command.Command(func(c *command.Context) error {
					return sendToServer(proxy, c, c.String(sendPlayerArg), c.String(sendServerArg))
				})),
			),
		)
}

func sendToServer(proxy *Proxy, c *command.Context, playerName, serverName string) error {
	if strings.EqualFold(playerName, "all") {
		return connectPlayersToServer(c, proxy, serverName, proxy.Players()...)
	}

	if strings.EqualFold(playerName, "current") {
		if player, ok := c.Source.(Player); ok {
			if currentServer := player.CurrentServer(); currentServer != nil {
				return connectPlayersToServer(c, proxy, serverName, PlayersToSlice[Player](currentServer.Server().Players())...)
			}
		} else {
			return c.Source.SendMessage(&Text{S: Style{Color: Red}, Content: "Only players can use 'current'!"})
		}
		return nil
	}

	player := proxy.PlayerByName(playerName)
	if player == nil {
		return c.Source.SendMessage(&Text{S: Style{Color: Red}, Content: fmt.Sprintf("Player %q doesn't exist.", playerName)})
	}

	return connectPlayersToServer(c, proxy, serverName, player)
}
func playerSuggestionProvider(proxy *Proxy, additionalPlayers ...string) brigodier.SuggestionProvider {
	return command.SuggestFunc(func(
		_ *command.Context,
		b *brigodier.SuggestionsBuilder,
	) *brigodier.Suggestions {
		candidates := append(playerNames(proxy), additionalPlayers...)
		return suggest.Similar(b, candidates).Build()
	})
}

func playerNames(proxy *Proxy) []string {
	list := proxy.Players()
	n := make([]string, len(list))
	for i, player := range list {
		n[i] = player.Username()
	}
	return n
}
