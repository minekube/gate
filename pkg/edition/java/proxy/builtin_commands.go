package proxy

import (
	"fmt"
	"strings"

	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/util/permission"
)

func (p *Proxy) registerBuiltinCommands() []string {
	return []string{
		p.command.Register(newServerCmd(p)).Name(),
		p.command.Register(newGlistCmd(p)).Name(),
		p.command.Register(newSendCmd(p)).Name(),
	}
}

// builtinCommandPermissions maps a builtin command's root literal to the
// permission node it requires. It is the single source for both the brigodier
// requirement and the "why was I refused" answer a player gets.
func builtinCommandPermissions() map[string]string {
	return map[string]string{
		"server": serverCmdPermission,
		"glist":  glistCmdPermission,
		"send":   sendCmdPermission,
	}
}

// builtinCmdAllowed reports whether the source may run a builtin command that
// requires the given permission node.
func builtinCmdAllowed(cfg *config.Config, source permission.Subject, node string) bool {
	return !cfg.RequireBuiltinCommandPermissions || source.HasPermission(node)
}

func hasCmdPerm(proxy *Proxy, perm string) brigodier.RequireFn {
	return command.Requires(func(c *command.RequiresContext) bool {
		return builtinCmdAllowed(proxy.config(), c.Source, perm)
	})
}

// deniedBuiltinCommandNode returns the permission node of the builtin command in
// the command line when the source may not run it, and "" when the line does not
// name a builtin command or the source is permitted.
//
// A builtin command refused by its requirement is indistinguishable from an
// unknown command at the dispatcher (brigodier drops the node from the parse),
// so this is what lets the proxy answer with a reason instead of forwarding the
// command to the backend, which can only reply "Unknown command".
func deniedBuiltinCommandNode(cfg *config.Config, source permission.Subject, commandline string) string {
	if !cfg.BuiltinCommands || source == nil {
		return ""
	}
	line := strings.TrimPrefix(strings.TrimSpace(commandline), "/")
	if i := strings.IndexAny(line, " \t"); i >= 0 {
		line = line[:i]
	}
	node, isBuiltin := builtinCommandPermissions()[line]
	if !isBuiltin || builtinCmdAllowed(cfg, source, node) {
		return ""
	}
	return node
}

// builtinCmdRefusedMessage is sent to a player who may not run a builtin command.
func builtinCmdRefusedMessage(node string) *component.Text {
	return &component.Text{
		Content: fmt.Sprintf("You don't have permission to use this command (requires %q).", node),
		S:       component.Style{Color: color.Red},
	}
}
