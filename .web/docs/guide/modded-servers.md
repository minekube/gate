---
title: 'Gate Minecraft Proxy with Modded Servers - Fabric & NeoForge'
description: 'Complete guide to using Gate with modded Minecraft servers including Fabric and NeoForge. Velocity modern forwarding, player info forwarding, and mod compatibility.'
---

# Modded Server Compatibility

Gate provides excellent compatibility with modded Minecraft servers including **Fabric** and **NeoForge**. This guide will help you set up Gate to work seamlessly with your modded servers.

## Overview

Gate provides comprehensive support for modded Minecraft servers, implementing the same forwarding protocols as described in the [Velocity documentation](https://docs.papermc.io/velocity/player-information-forwarding/):

### Forwarding Modes Supported

- **Velocity modern forwarding** - Secure binary format with MAC authentication (Minecraft 1.13+)
- **Legacy BungeeCord forwarding** - Compatible with older versions and servers
- **BungeeGuard forwarding** - Enhanced security over legacy forwarding
- **No forwarding** - Basic proxy functionality without player data forwarding

## Fabric Server Setup

Gate works with Fabric out of the box, but you should add support for player info forwarding using a mod like [FabricProxy-Lite](https://modrinth.com/mod/fabricproxy-lite) (which supports Velocity modern forwarding).

In addition, if you intend to run mods that add new content on top of Vanilla, you should install [CrossStitch](https://modrinth.com/mod/crossstitch), which improves support for certain Minecraft features that are extended by mods, such as custom argument types. This mod is officially maintained by the Velocity project.

### Required Mods

#### FabricProxy-Lite (For Velocity Forwarding)

- **Purpose**: Enables Velocity modern forwarding for Fabric servers
- **Download**: [FabricProxy-Lite on Modrinth](https://modrinth.com/mod/fabricproxy-lite)
- **Requires**: [Fabric API](https://modrinth.com/mod/fabric-api) (dependency)

::: warning Version Compatibility
Always download the mod version that matches your Minecraft server version. Check the compatibility table on the mod page.
:::

#### CrossStitch (Recommended for Modded Content)

- **Purpose**: Improves support for Minecraft features extended by mods (custom argument types, etc.)
- **Download**: [CrossStitch on Modrinth](https://modrinth.com/mod/crossstitch)
- **Maintained by**: Velocity project (official)

### Server Configuration

::: code-group

```properties [server.properties]
server-port=25566
online-mode=false // [!code ++]
motd=Fabric Server with Gate Proxy
```

```toml [config/FabricProxy-Lite.toml]
hackOnlineMode = false // [!code ++]
hackEarlySend = false // [!code ++]
hackMessageChain = false // [!code ++]
disconnectMessage = "This server requires you to connect with Gate."
secret = "your-secret-key-here" // [!code ++]
```

```yaml [Gate config.yml]
config:
  bind: 0.0.0.0:25565
  servers:
    fabric-server: localhost:25566
  try:
    - fabric-server
  forwarding:
    mode: velocity // [!code ++]
    velocitySecret: 'your-secret-key-here' // [!code ++]
  status:
    motd: |
      §bGate Proxy with Fabric
      §eVelocity Forwarding Enabled
```

:::

::: tip Configuration File
If `FabricProxy-Lite.toml` doesn't appear in the `config/` directory after server restart, create it manually or use environment variables:

**Environment Variables Alternative:**

```bash
export FABRIC_PROXY_SECRET="your-secret-key-here"
export FABRIC_PROXY_HACK_ONLINE_MODE=false
export FABRIC_PROXY_HACK_EARLY_SEND=false
export FABRIC_PROXY_HACK_MESSAGE_CHAIN=false
```

:::

## NeoForge Server Setup

Gate works with NeoForge out of the box, but you should add support for player info forwarding using a mod like [Proxy-Compatible-Forge](https://github.com/adde0109/Proxy-Compatible-Forge) (which supports Velocity modern forwarding).

### Required Mods

#### Proxy-Compatible-Forge (For Velocity Forwarding)

- **Purpose**: Enables Velocity modern forwarding for NeoForge servers
- **Download**: [Proxy-Compatible-Forge on GitHub](https://github.com/adde0109/Proxy-Compatible-Forge/releases)
- **Supports**: NeoForge 1.16.5 - 1.20.x+

::: warning Version Compatibility
Ensure you download the correct mod version for your NeoForge server version. Check the releases page for compatibility information.
:::

### Server Configuration

::: code-group

```properties [server.properties]
server-port=25567
online-mode=false // [!code ++]
motd=NeoForge Server with Gate Proxy
```

```toml [config/pcf-common.toml]
#Modern Forwarding Settings
[modernForwarding]
    forwardingSecret = "your-secret-key-here" // [!code ++]

[commandWrapping]
    #List of argument types that are not vanilla but are integrated into the server
    moddedArgumentTypes = ["livingthings:sampler_types"]
```

```yaml [Gate config.yml]
config:
  bind: 0.0.0.0:25565
  servers:
    neoforge-server: localhost:25567
  try:
    - neoforge-server
  forwarding:
    mode: velocity // [!code ++]
    velocitySecret: 'your-secret-key-here' // [!code ++]
  status:
    motd: |
      §bGate Proxy with NeoForge
      §eVelocity Forwarding Enabled
```

:::

## Multi-Server Setup

You can run both Fabric and NeoForge servers behind the same Gate proxy:

::: code-group

```yaml [Gate config.yml]
config:
  bind: 0.0.0.0:25565
  servers:
    fabric-server: localhost:25566 # Fabric server
    neoforge-server: localhost:25567 # NeoForge server
    vanilla-server: localhost:25568 # Vanilla server
  try:
    - fabric-server
    - neoforge-server
    - vanilla-server
  forwarding:
    mode: velocity // [!code ++]
    velocitySecret: 'shared-secret-key' // [!code ++]
  status:
    motd: |
      §bGate Multi-Server Network
      §eFabric • NeoForge • Vanilla
```

:::

## Troubleshooting

### Common Issues

#### Connection Refused

- **Check server ports** - ensure servers are running on configured ports
- **Verify firewall** - make sure ports are accessible
- **Check server logs** - look for startup errors

#### Forwarding Issues

- **Secret mismatch** - ensure `velocitySecret` matches in both Gate and mod configs
- **Online mode** - must be `false` on backend servers when using forwarding
- **Mod compatibility** - verify the forwarding mod supports your server version

### Mod Compatibility Notes

#### Incompatible Mods

Some older mods may not work with current NeoForge versions:

- **NeoVelocity 1.2.4** - Incompatible with NeoForge 21.8.x
- **NeoForwarding 1.3.0** - Only supports older NeoForge versions

### Security Considerations

When running modded servers:

- **Use velocity forwarding** when possible for better security
- **Configure firewalls** to block direct access to backend servers
- **Regular updates** - keep Gate and mods updated

## Getting Help

If you encounter issues:

1. **Check the logs** - both Gate and server logs
2. **Verify versions** - ensure compatibility between Gate, server, and mods
3. **Community support** - join the [Gate Discord](https://minekube.com/discord)
4. **GitHub issues** - If you encounter a reproducible bug or have a technical issue that includes logs, error messages, or steps to reproduce, please report it on the [Gate repository](https://github.com/minekube/gate/issues). For general questions or troubleshooting without technical details, use the [Gate Discord](https://minekube.com/discord).

---

_This guide covers the most common modded server setups. For specific mod compatibility questions, consult the mod's documentation or community._
