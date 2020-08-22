package proxy

import (
	"context"
	"fmt"
	. "go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/proxy/permission"
	"time"
)

var missingCommandPermission = &Text{
	Content: "You do not have the permission to run this command.",
	S:       Style{Color: Red}}

func (p *Proxy) registerBuiltinCommands() {
	p.command.Register(&serverCmd{proxy: p}, "server")
}

func hasCmdPerm(s CommandSource, perm string) bool {
	if s.PermissionValue(perm) == permission.False {
		_ = s.SendMessage(missingCommandPermission)
		return false
	}
	return true
}

//
//
//
//

const serverCmdPermission = "gate.command.server"

type serverCmd struct{ proxy *Proxy }

func (s *serverCmd) Invoke(c *Context) {
	if !hasCmdPerm(c.Source, serverCmdPermission) {
		return
	}

	if len(c.Args) == 0 {
		s.list(c)
		return
	}
	s.connect(c)
}

// switch server
func (s *serverCmd) connect(c *Context) {
	player, ok := c.Source.(Player)
	if !ok {
		_ = c.Source.SendMessage(&Text{Content: "Only players can connect to a server!", S: Style{Color: Red}})
		return
	}

	server := c.Args[0]
	rs := s.proxy.Server(server)
	if rs == nil {
		_ = c.Source.SendMessage(&Text{Content: fmt.Sprintf("Server %q not registered", server), S: Style{Color: Red}})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(s.proxy.config.ConnectionTimeout))
	defer cancel()
	player.CreateConnectionRequest(rs).ConnectWithIndication(ctx)
}

// list registered servers
func (s *serverCmd) list(c *Context) {
	const maxEntries = 50
	var servers []Component
	proxyServers := s.proxy.Servers()
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
	_ = c.Source.SendMessage(&Text{
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
}
