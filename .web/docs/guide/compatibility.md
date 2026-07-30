---
title: "Gate Compatibility"
description: "Gate compatibility notes for Minecraft server implementations, modded servers, proxy setups, Bedrock players, and multi-version networks."
---

# Compatibility

Gate is compatible with many Minecraft server implementations. If the server
acts like vanilla, Gate should work, and Gate includes specific support for
common modded setups.

Gate itself supports Minecraft 1.8 through the latest version supported by the
current Gate release. For client/backend protocol translation between different
Java Minecraft versions, see [Multi-Version Support](multi-version).

## Server Implementations

### Paper <VPBadge>Recommended</VPBadge>

We highly recommend using [Paper](https://papermc.io/) for running a server in most cases.
Gate is tested with older and most recent versions of it.

You can use `modern` forwarding (like Velocity does) if you run Paper
1.13.2 or higher. If you use Paper 1.12.2 or lower, you can use `legacy` BungeeCord-style forwarding or the more secure `bungeeguard` [Bungeeguard](https://www.spigotmc.org/resources/bungeeguard.79601/) forwarding.

### Spigot

Spigot is not well-tested with Gate.
However, it is based on vanilla and as it is the base for Paper, it is relatively well-supported.

Spigot does not support Gate/Velocity modern forwarding, but does support legacy BungeeCord forwarding and the more secure Bungeeguard forwarding when using the [Bungeeguard](https://www.spigotmc.org/resources/bungeeguard.79601/) plugin.

### Forks of Spigot/Paper

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

### Forge 1.13–1.20.1 (FML2/FML3) <VPBadge>Fully Supported</VPBadge>

- **All forwarding modes** - Velocity modern forwarding (with [PCF](https://modrinth.com/mod/proxy-compatible-forge)), BungeeCord, and BungeeGuard
- **Built-in FML login relay** - Gate relays `fml:loginwrapper` LoginPluginMessages during the LOGIN phase, similar to what [Ambassador](https://modrinth.com/plugin/ambassador) does for Velocity
- **No client-side mods required** - The player doesn't need any special mods
- **Server switch support** - Cached FML responses are replayed for compatible server switches

### Legacy Forge (1.8–1.12.2)

- **Limited support** - Basic functionality works
- **Legacy forwarding only** - Use BungeeCord forwarding
- **Older versions** - May have compatibility issues

For setup instructions, configuration examples, and troubleshooting, see the
[Modded Servers Guide](modded-servers).

## Bedrock Players

Bedrock support is handled by [GeyserLite](/geyserlite/) before the player
reaches Gate. Start with the [Bedrock support guide](bedrock) for setup.

If Bedrock players need to join Java backend servers on a different Minecraft
version, Gate may also use [Multi-Version Support](multi-version) behind Gate,
after GeyserLite has translated the Bedrock session into Java protocol.

## Proxy-Behind-Proxy Setups

These setups are only supported with [Lite mode](lite#proxy-behind-proxy) or
[Connect enabled](connect).

Avoid other proxy-behind-proxy setups where possible. They often introduce
ambiguous authentication and forwarding behavior that is better handled by
purpose-built solutions like [Connect](https://connect.minekube.com/) or
[Lite mode](lite).
