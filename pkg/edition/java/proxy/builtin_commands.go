package proxy

import (
	"go.minekube.com/brigodier"
	"go.minekube.com/gate/pkg/command"
)

func (p *Proxy) registerBuiltinCommands() {
	p.command.Register(newServerCmd(p))
	p.command.Register(newGlistCmd(p))
}

func hasCmdPerm(proxy *Proxy, perm string) brigodier.RequireFn {
	return command.Requires(func(c *command.RequiresContext) bool {
		return !proxy.cfg.RequireBuiltinCommandPermissions || c.Source.HasPermission(perm)
	})
}
