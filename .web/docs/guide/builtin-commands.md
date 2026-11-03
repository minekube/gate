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

Gate ships no permission provider and has no permission file by default. A player's permissions are whatever function the
`proxy.PermissionsSetupEvent` handler sets, and that event is fired once per player at login. Until something handles
it, a player's permissions are *undefined*, which counts as denied - so `requireBuiltinCommandPermissions: true` on its
own denies **all** built-in commands to every player.

You can either answer the event from a Go extension, or let Gate answer it from the config (the next section).

### Grant permissions in the config

The `permissions` block is a file-backed permission provider that Gate itself answers `proxy.PermissionsSetupEvent`
with, so the common "players may use `/server`, but `/send` stays restricted" setup needs no extension:

```yaml
permissions:
  default:                      # granted to every player with no entry below
    - gate.command.server
  players:                      # a listed player REPLACES default
    "<uuid>":
      - gate.command.server
      - gate.command.glist
    "Notch":
      - gate.command.*
  deny:                         # explicit denials win over every grant
    "Notch":
      - gate.command.send
```

It only has an effect together with `requireBuiltinCommandPermissions: true`, which makes the built-in commands check
the nodes listed in the [commands table](#commands). A player that lacks a node is told why instead of having the
command forwarded to the backend:

> You don't have permission to use this command (requires "gate.command.send").

Semantics:

- `default` is granted to every player that has no entry under `players`.
- `players` **replaces** `default` for a listed player: an entry grants exactly the nodes it lists, so a listed player
  that is not granted `gate.command.server` cannot use `/server` even when `default` grants it.
- `deny` always wins, over `default` and over `players`.
- Entries are permission nodes (`gate.command.server`), a trailing wildcard prefix (`gate.command.*`) or `*`.
- Keys are player UUIDs; usernames are accepted case-insensitively as a convenience only, because an offline-mode name
  is not an identity. Prefer UUIDs.
- An invalid key or pattern fails config validation at startup instead of silently not matching, and an empty
  `permissions` block changes nothing.
- The block is read when the proxy starts; a config reload does not re-register the provider yet.
- Only players are affected: the console and other non-player command sources are untouched, and a node the block does
  not mention stays undecided, so other providers still decide it.

### Grant permissions with an extension

For anything beyond a static file - groups, a database, LuckPerms-style inheritance - register a
[Gate extension](/developers/) whose plugin answers the event:

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

An extension that answers the event after Gate's config provider (the plugin init hooks run after it) replaces it, so
the two can be combined: grant the shared nodes in `permissions` and let the extension decide the rest.

The [plugin template](https://github.com/minekube/gate-plugin-template) is the quickest starting point for an
extension like this; its
[`plugins/permissions`](https://github.com/minekube/gate-plugin-template/tree/main/plugins/permissions) extension is a
file-backed provider reading its own `permissions.yml`, for setups that need a provider file separate from Gate's
config.

## Disable built-in commands

By default, built-in command are registered on startup.
You can change this behaviour by setting `builtinCommands: false` in the config.
