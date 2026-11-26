package proxy

import (
	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/util/permission"
	"go.minekube.com/gate/pkg/util/uuid"
)

// permissionIdentity is the player identity config.permissions selects entries
// by. Players implement it; console and other non-player subjects do not, which
// is how a file-backed provider stays scoped to players.
type permissionIdentity interface {
	ID() uuid.UUID
	Username() string
}

// configPermissionsPriority is the priority Gate's own config.permissions
// provider subscribes to PermissionsSetupEvent with.
//
// setupConfigPermissions runs before the plugin init hooks (Proxy.init precedes
// initPlugins), so an extension that answers the event afterwards replaces the
// provider with its own SetFunc - that escape hatch is deliberate and is why the
// provider must not claim a higher priority than plugins use.
const configPermissionsPriority = 0

// setupConfigPermissions registers the file-backed permission provider from
// config.permissions.
//
// An empty permissions block registers nothing, which keeps today's behaviour
// byte for byte: no provider, every node undefined, and undefined counts as
// denied only when requireBuiltinCommandPermissions is on.
//
// The block is read when the proxy starts; a config reload does not re-register
// the provider (a follow-up on minekube/gate#1293).
func (p *Proxy) setupConfigPermissions() {
	set, err := p.config().Permissions.Build()
	if err != nil {
		// Config validation rejects an invalid block at startup. Register nothing
		// rather than guess at permissions.
		p.log.Error(err, "ignoring invalid config.permissions")
		return
	}
	if set == nil {
		return
	}
	event.Subscribe(p.event, configPermissionsPriority, func(e *PermissionsSetupEvent) {
		identity, ok := e.Subject().(permissionIdentity)
		if !ok {
			return // console or another non-player subject: unaffected
		}
		// Nodes the block does not know stay undecided for whoever handled the
		// event before this provider (e.g. an extension at a higher priority).
		previous := e.Func()
		id, username := identity.ID(), identity.Username()
		e.SetFunc(func(node string) permission.TriState {
			if value := set.ValueFor(id, username, node); value != permission.Undefined {
				return value
			}
			if previous == nil {
				return permission.Undefined
			}
			return previous(node)
		})
	})
}
