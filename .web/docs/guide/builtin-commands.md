---
title: "Gate Built-in Minecraft Proxy Commands"
description: "Learn about Gate's built-in commands like /server, /glist, /send. Configure permissions and manage players across your Minecraft server network."
---

# Builtin Commands

Gate includes a few generally useful built-in commands by default.

If you want to add custom commands refer to the [Developers Guide](/developers/).


## Commands

| Built-In Command | Permission            | Description                                                                             |
|------------------|-----------------------|-----------------------------------------------------------------------------------------|
| `/server`        | `gate.command.server` | Players can use the command to view and switch to another server.                       |
| `/glist`         | `gate.command.glist`  | View the number of players on the Gate instance. `/glist all` lists players per server. |
| `/send`          | `gate.command.send`   | Send one or all players to another server.                                              |

## Permission

By default, the built-in commands don't require the listed permissions.
You can change this behaviour by setting `requireBuiltinCommandPermissions: true` in the config.

It is useful if you only want to allow players with certain permissions to use the commands.

### Where do permissions come from?

Gate ships no permission provider and has no permission file. A player's permissions are whatever function the
`proxy.PermissionsSetupEvent` handler sets, and that event is fired once per player at login. Until something handles
it, a player's permissions are *undefined*, which counts as denied - so `requireBuiltinCommandPermissions: true` on its
own denies **all** built-in commands to every player.

To grant them, register a [Gate extension](/developers/) whose plugin answers the event:

```go
var Plugin = proxy.Plugin{
	Name: "Permissions",
	Init: func(ctx context.Context, p *proxy.Proxy) error {
		event.Subscribe(p.Event(), 0, func(e *proxy.PermissionsSetupEvent) {
			player, ok := e.Subject().(proxy.Player)
			if !ok {
				return
			}
			e.SetFunc(func(node string) permission.TriState {
				// Replace this with your own lookup: a config file, a database,
				// your own permission plugin, and so on.
				if player.Username() == "Notch" && node == "gate.command.server" {
					return permission.True
				}
				return permission.Undefined
			})
		})
		return nil
	},
}
```

Return `permission.True` to allow a node, `permission.False` to deny it explicitly, and `permission.Undefined` to leave
it undecided - for the built-in commands undefined counts as denied, but another provider can still decide it.

The [plugin template](https://github.com/minekube/gate-plugin-template) is the quickest starting point for an
extension like this.

## Disable built-in commands

By default, built-in command are registered on startup.
You can change this behaviour by setting `builtinCommands: false` in the config.
