---
title: "Gate Compatibility - Supported Minecraft Versions"
description: "Gate supports Minecraft versions 1.8 to latest, including modded servers, plugins, and cross-platform compatibility."
---

# Server Compatibility

Gate is compatible with many Minecraft server implementations.
The expectation is that if the server acts like vanilla, Gate will work,
and we make special provisions for modded setups where we can.

Gate provides excellent support for **modded servers** including Fabric and NeoForge. See our [Modded Servers Guide](modded-servers) for detailed setup instructions.

Gate is compatible with Minecraft 1.8 through the latest version
and we maintainers update Gate as soon as a new Minecraft version is released.

## Paper <VPBadge>Recommended</VPBadge>

We highly recommend using [Paper](https://papermc.io/) for running a server in most cases.
Gate is tested with older and most recent versions of it.

You can use `modern` forwarding (like Velocity does) if you run Paper
1.13.2 or higher. If you use Paper 1.12.2 or lower, you can use `legacy` BungeeCord-style forwarding or the more secure `bungeeguard` [Bungeeguard](https://www.spigotmc.org/resources/bungeeguard.79601/) forwarding.

## Spigot

Spigot is not well-tested with Gate.
However, it is based on vanilla and as it is the base for Paper, it is relatively well-supported.

Spigot does not support Gate/Velocity modern forwarding, but does support legacy BungeeCord forwarding and the more secure Bungeeguard forwarding when using the [Bungeeguard](https://www.spigotmc.org/resources/bungeeguard.79601/) plugin.

## Forks of Spigot/Paper

Forks of Spigot are not well-tested with Gate, though they may work perfectly fine.

## Modded Servers <VPBadge>Supported</VPBadge>

Gate has excellent compatibility with modded Minecraft servers:

### Fabric <VPBadge>Fully Supported</VPBadge>

- **Native compatibility** - Works out of the box
- **Velocity modern forwarding** - Using FabricProxy-Lite mod
- **All Fabric versions** - 1.16+ supported
- **Mod compatibility** - Extensive testing with popular mods

### NeoForge <VPBadge>Fully Supported</VPBadge>

- **Velocity modern forwarding** - Using Proxy-Compatible-Forge mod
- **Full protocol support** - 1.20.x+ compatibility
- **Command support** - Proper handling of modded commands
- **Cross-version support** - Works with various Minecraft versions

### Legacy Forge

- **Limited support** - Basic functionality works
- **Legacy forwarding only** - Use BungeeCord forwarding
- **Older versions** - 1.12.2 and below may have compatibility issues

For detailed setup instructions, configuration examples, and troubleshooting, see our comprehensive [Modded Servers Guide](modded-servers).

## Proxy-behind-proxy setups

These setups are only supported with [Lite mode](lite#proxy-behind-proxy) or [Connect enabled](connect)!

You are best advised to avoid other kinds of setups, as they can cause lots of issues.
Most proxy-behind-proxy setups are either illogical in the first place or can be handled more
gracefully by purpose-built solutions like [Connect](https://connect.minekube.com/) or [Lite mode](lite).
