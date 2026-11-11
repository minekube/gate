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

// builtinCmdRefusedMessageNodeToken is replaced with the permission node the
// refused command requires.
const builtinCmdRefusedMessageNodeToken = "{node}"

// builtinCmdRefusedMessage is sent to a player who may not run a builtin
// command. The message is configurable (config.builtinCommandRefusedMessage);
// without one, the built-in default is used.
func builtinCmdRefusedMessage(cfg *config.Config, node string) *component.Text {
	if cfg == nil || cfg.BuiltinCommandRefusedMessage == nil {
		return defaultBuiltinCmdRefusedMessage(node)
	}
	// The live config keeps its template: the placeholder is substituted into a
	// copy, per player.
	return substituteRefusedMessageNode(cfg.BuiltinCommandRefusedMessage.T(), node)
}

// substituteRefusedMessageNode returns a copy of t with every {node} token
// replaced. The token is substituted in each text node, not only the root: a
// configured value in legacy '§' format is decoded into a root Text whose
// visible text sits in Extra, so a root-only replacement would leave the
// placeholder on screen.
func substituteRefusedMessageNode(t *component.Text, node string) *component.Text {
	out := *t
	out.Content = strings.ReplaceAll(t.Content, builtinCmdRefusedMessageNodeToken, node)
	if len(t.Extra) != 0 {
		out.Extra = make([]component.Component, len(t.Extra))
		for i, child := range t.Extra {
			if childText, ok := child.(*component.Text); ok {
				out.Extra[i] = substituteRefusedMessageNode(childText, node)
				continue
			}
			out.Extra[i] = child
		}
	}
	return &out
}

// defaultBuiltinCmdRefusedMessage is the refusal Gate sends when no message is
// configured: the exact text it always answered with.
func defaultBuiltinCmdRefusedMessage(node string) *component.Text {
	return &component.Text{
		Content: fmt.Sprintf("You don't have permission to use this command (requires %q).", node),
		S:       component.Style{Color: color.Red},
	}
}
