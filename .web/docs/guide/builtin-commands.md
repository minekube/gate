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

## Disable built-in commands

By default, built-in command are registered on startup.
You can change this behaviour by setting `builtinCommands: false` in the config.
