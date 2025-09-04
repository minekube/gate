---
title: 'Gate ForcedHosts - Domain-Based Server Routing'
description: 'Configure Gate ForcedHosts to route players to specific servers based on the hostname they connect with. Perfect for multi-server networks with different domains.'
---

# ForcedHosts Configuration

ForcedHosts allows you to route players to specific servers based on the **hostname** they use to connect to your Gate proxy. This is perfect for multi-server networks where you want different domains to lead to different game modes or servers.

## How It Works

When a player connects to your Gate proxy, Gate examines the hostname they used (e.g., `creative.example.com`) and routes them to the appropriate server based on your ForcedHosts configuration.

::: info Classic Mode Feature
ForcedHosts is available in **classic mode** (when `lite.enabled: false`). For lightweight deployments, see [Gate Lite Mode](/guide/lite) which provides similar host-based routing functionality.
:::

### Key Features

- **Hostname-only matching** - Port numbers are ignored
- **Case-insensitive** - `Creative.Example.com` matches `creative.example.com`
- **Load balancing** - Multiple servers per hostname for distribution
- **Fallback support** - Uses `try` list when no forced host matches
- **Virtual host cleaning** - Handles Forge separators and TCPShield automatically

## Basic Configuration

:::: code-group

```yaml [Simple Domain Routing]
config:
  servers:
    lobby: localhost:25565
    creative: localhost:25566
    survival: localhost:25567
    minigames: localhost:25568

  try:
    - lobby # Default server for unmatched hostnames

  forcedHosts:
    'creative.example.com': ['creative']
    'survival.example.com': ['survival']
    'games.example.com': ['minigames']
    'lobby.example.com': ['lobby']
```

```yaml [Load Balancing Setup]
config:
  servers:
    lobby-1: localhost:25565
    lobby-2: localhost:25566
    creative-1: localhost:25567
    creative-2: localhost:25568
    survival: localhost:25569

  try:
    - lobby-1
    - lobby-2

  forcedHosts:
    # Load balance between multiple lobby servers
    'lobby.example.com': ['lobby-1', 'lobby-2']

    # Load balance between multiple creative servers
    'creative.example.com': ['creative-1', 'creative-2']

    # Single survival server
    'survival.example.com': ['survival']
```

::::

## Advanced Examples

:::: code-group

```yaml [Multi-Domain Network]
config:
  servers:
    hub: localhost:25565
    skyblock: localhost:25566
    prison: localhost:25567
    creative: localhost:25568
    survival: localhost:25569

  try:
    - hub

  forcedHosts:
    # Main network domains
    'play.mynetwork.com': ['hub']
    'hub.mynetwork.com': ['hub']

    # Game mode specific domains
    'skyblock.mynetwork.com': ['skyblock']
    'prison.mynetwork.com': ['prison']
    'creative.mynetwork.com': ['creative']
    'survival.mynetwork.com': ['survival']

    # Alternative domains
    'sb.mynetwork.com': ['skyblock'] # Short alias
    'build.mynetwork.com': ['creative'] # Alternative name
```

```yaml [Development and Production]
config:
  servers:
    prod-lobby: localhost:25565
    prod-survival: localhost:25566
    dev-lobby: localhost:25567
    dev-survival: localhost:25568

  try:
    - prod-lobby

  forcedHosts:
    # Production domains
    'play.example.com': ['prod-lobby']
    'survival.example.com': ['prod-survival']

    # Development domains
    'dev.example.com': ['dev-lobby']
    'dev-survival.example.com': ['dev-survival']

    # Testing domains
    'test.example.com': ['dev-lobby']
```

```yaml [Mixed Strategies]
config:
  servers:
    lobby-1: localhost:25565
    lobby-2: localhost:25566
    creative-main: localhost:25567
    creative-backup: localhost:25568
    survival: localhost:25569
    minigames-1: localhost:25570
    minigames-2: localhost:25571

  try:
    - lobby-1
    - lobby-2

  forcedHosts:
    # Load balanced lobbies
    'lobby.example.com': ['lobby-1', 'lobby-2']

    # Primary with backup
    'creative.example.com': ['creative-main', 'creative-backup']

    # Single server
    'survival.example.com': ['survival']

    # Multiple game servers
    'games.example.com': ['minigames-1', 'minigames-2']
```

::::

## DNS Configuration

To use ForcedHosts effectively, you need to configure your DNS records to point all your domains to your Gate proxy:

::: code-group

```dns [Individual Records]
# DNS A Records
play.example.com        A    YOUR_SERVER_IP
creative.example.com    A    YOUR_SERVER_IP
survival.example.com    A    YOUR_SERVER_IP
lobby.example.com       A    YOUR_SERVER_IP
```

```dns [Wildcard Record]
# Wildcard (if supported by your DNS provider)
*.example.com           A    YOUR_SERVER_IP

# Still need the root domain
example.com             A    YOUR_SERVER_IP
```

:::

## Behavior Examples

Given this configuration:

```yaml
forcedHosts:
  'creative.example.com': ['creative-server']
  'survival.example.com': ['survival-server']

try:
  - lobby-server
```

### Connection Examples

| Player Connects To           | Routes To         | Reason                         |
| ---------------------------- | ----------------- | ------------------------------ |
| `creative.example.com:25565` | `creative-server` | Matches forced host            |
| `CREATIVE.EXAMPLE.COM:25565` | `creative-server` | Case-insensitive match         |
| `survival.example.com:12345` | `survival-server` | Port ignored, hostname matches |
| `other.example.com:25565`    | `lobby-server`    | No match, uses try list        |
| `192.168.1.100:25565`        | `lobby-server`    | IP address, uses try list      |

## Troubleshooting

### Common Issues

::: details Players Not Routing Correctly

**Problem**: Players connecting to `creative.example.com` end up on the wrong server.

**Solutions**:

1. **Check DNS**: Ensure your domain points to the Gate proxy IP
2. **Verify configuration**: Make sure the hostname in `forcedHosts` matches exactly
3. **Check server names**: Ensure server names in `forcedHosts` exist in `servers` section
4. **Test case sensitivity**: Try lowercase hostnames in your config

:::

::: details Load Balancing Not Working

**Problem**: All players go to the same server despite multiple servers listed.

**Solutions**:

1. **Check server availability**: Ensure all servers in the list are online
2. **Verify server addresses**: Make sure all servers are reachable
3. **Check logs**: Look for connection errors to specific servers

:::

::: details Fallback Not Working

**Problem**: Players get disconnected when no forced host matches.

**Solutions**:

1. **Configure try list**: Ensure you have a `try` list with available servers
2. **Check server status**: Make sure fallback servers are online
3. **Verify server names**: Ensure servers in `try` list exist in `servers` section

## Migration from Broken Configs

If you had ForcedHosts configured before PR #560 but it wasn't working, your configuration should now work automatically! The fix included:

- **Hostname extraction** - Now properly strips ports and handles virtual host cleaning
- **Case normalization** - All matching is now case-insensitive
- **Proper fallback** - Correctly uses `try` list when no forced host matches

No configuration changes are needed - your existing `forcedHosts` should start working immediately after updating Gate.

## Related Features

- **[Gate Lite Mode](/guide/lite)** - Alternative host-based routing for lightweight deployments
- **[Server Configuration](/guide/config/)** - Complete server and proxy configuration
- **[Load Balancing](/guide/config/#load-balancing)** - Advanced load balancing strategies
