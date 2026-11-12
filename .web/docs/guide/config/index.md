---
title: 'Gate Configuration Guide - Proxy Settings'
description: 'Configure Gate Minecraft proxy with YAML settings, server routes, authentication, and advanced proxy features.'
---

# Configuration

Gate uses a YAML configuration file (`config.yml`) to configure all proxy settings.

## Config File Location

Gate looks for `config.yml` in the current working directory by default. Use `--config` or `-c` to specify a custom path:

```sh console
$ gate                    # Uses ./config.yml
$ gate -c /path/to/config.yml
```

Gate supports YAML (`.yml`, `.yaml`), JSON (`.json`), and environment variables with `GATE_` prefix. You can mix formats - use a config file and override values with environment variables.

## Configuration Templates

Generate configuration files using the `gate config` command, or create them manually:

```sh console
# Write to config.yml
$ gate config --write
$ gate config --type <type> --write

# Write to custom file using pipe redirection
$ gate config > my-config.yml
$ gate config --type <type> > my-config.yml
```

You can also create the `config.yml` file manually using any of the templates below as a starting point.

::: code-group

```yaml [Full (default)]
<!--@include: ../../../../config.yml -->
```

```yaml [Simple]
<!--@include: ../../../../config-simple.yml -->
```

```yaml [Lite]
<!--@include: ../../../../config-lite.yml -->
```

```yaml [Bedrock]
<!--@include: ../../../../config-bedrock.yml -->
```

```yaml [Minimal]
<!--@include: ../../../../pkg/configs/config-minimal.yml -->
```

:::

For most users, the full configuration is recommended. You can generate it and then edit the `servers` section to point to your backend Minecraft servers.

## Server List Appearance

In full proxy mode Gate answers the server list ping itself, so the entry players see in their client
comes from the `status` block - not from a backend. The keys, all of them optional:

```yaml
status:
  # Legacy '§' format or a modern JSON text component.
  motd: |-
    §bA Gate Proxy
  # Displayed player count; this is not a player limit.
  showMaxPlayers: 1000
  # The version name shown in the client's server list.
  # Empty (the default) advertises Gate's supported version range.
  versionName: "Minekube Network"
  # A 64x64 image: a file path or a base64 data URI.
  favicon: favicon.png
  logPingRequests: false
```

- **`versionName`** (v0.77.0 and newer) replaces the version text only. Left empty, Gate advertises its
  supported range, e.g. `Gate 1.7.2-1.7.5-26.3`. The advertised `protocol` and the set of client versions
  Gate accepts are untouched by it, so a custom name never changes who can join.
- **`showMaxPlayers`** is display text as well: it does not limit how many players Gate accepts. Use
  `quota`/`packetLimiter` for admission control instead.
- **`motd`** accepts legacy `§` colours or a JSON text component, the same shape as `shutdownReason`.
- **`favicon`** is a path to a 64x64 PNG or a base64 data URI.

In [Lite mode](/guide/lite) the ping is proxied from the backend, so this block is inert and players see
the backend's MOTD and player count - see [modded servers](/guide/modded-servers) for the chained case.

