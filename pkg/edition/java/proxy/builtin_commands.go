package proxy

import (
	"go.minekube.com/brigodier"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/brigadier"
)

func (p *Proxy) registerBuiltinCommands() []string {
	return []string{
		p.command.Register(newServerCmd(p)).Name(),
		p.command.Register(newGlistCmd(p)).Name(),
		p.command.Register(newSendCmd(p)).Name(),
		p.command.Register(newTestEntityCmd(p)).Name(),
	}
}

func hasCmdPerm(proxy *Proxy, perm string) brigodier.RequireFn {
	return command.Requires(func(c *command.RequiresContext) bool {
		return !proxy.cfg.RequireBuiltinCommandPermissions || c.Source.HasPermission(perm)
	})
}

func newTestEntityCmd(proxy *Proxy) brigodier.LiteralNodeBuilder {
	return brigodier.Literal("testentity").
		Then(brigodier.Argument("player", brigadier.PlayerArgument).
			Executes(command.Command(func(c *command.Context) error {
				playerName := c.String("player")
				return c.SendMessage(&Text{
					Content: "EntityArgumentType: You specified player '" + playerName + "' - this uses proper minecraft:entity argument type!",
				})
			})))
}
