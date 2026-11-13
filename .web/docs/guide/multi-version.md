---
title: "Gate Multi-Version Support"
description: "Use Gate's managed Via support to let Java clients join backend servers across different Minecraft protocol versions."
---

# Multi-Version Support

Gate classic can route backend connections through managed Via-powered Java
protocol translation. This lets clients join backend servers even when the
client and backend Minecraft versions differ.

Enable it with:

```yaml
config:
  via:
    enabled: true
```

Gate starts [ViaLite](/vialite/) and routes backend connections through it.
This applies to configured servers and API-registered servers, so dynamic
Connect/session backends can use the same version translation path.

## Topology

```text
Java player
  -> Gate classic
  -> ViaLite
  -> backend server

Bedrock player
  -> GeyserLite
  -> Gate classic
  -> ViaLite
  -> backend server
```

ViaLite sits behind Gate. Gate accepts the player, handles routing and events,
and then ViaLite translates the backend-facing Java protocol where needed.

## Bedrock Players

[GeyserLite](/geyserlite/) translates Bedrock sessions into Java protocol
before the player reaches Gate. ViaLite can then help with Java backend version
differences behind Gate.

ViaLite does not replace Geyser protocol support. If Geyser does not support a
new Bedrock or Java version yet, production networks should usually wait for
official upstream Geyser support before upgrading Bedrock-critical backends.

## Gate Lite

Gate Lite raw-pipes backend traffic after the initial handshake so backend
servers keep authentication ownership. Managed Via translation needs to decode
and rewrite packets after login, so it belongs to Gate classic backend
connections, not Gate Lite.

## Runtime Details

For runtime modes, platform support, pins, offline deployment, and release
policy, see [ViaLite](/vialite/).

## Which ViaLite Version Runs

Managed ViaLite resolves its runtime **at Gate startup** and defaults to the
newest stable ViaLite release, so **restarting Gate applies a newly published
runtime** - there is no hot swap and no Gate upgrade needed to pick up new
client-version support. Pin or override it when you need a controlled rollout:

```yaml
config:
  via:
    enabled: true
    # version: v0.3.1     # pin an exact ViaLite release
    # binaryPath: /usr/local/bin/vialite
    # offline: true       # never download a runtime
    # mirror: https://mirror.example.com/vialite
```

Gate logs the runtime it resolved and where it came from
(`vialite: resolved runtime ... version=... source=download|cache|binaryPath|...`)
on every start, and the runtime itself reports the newest client version it can
translate. Details, cache location, and mirror behaviour:
[ViaLite runtime configuration](/vialite/#which-runtime-runs).

Because ViaVersion ships inside the ViaLite runtime, a Minecraft version that
ViaVersion supports only in a newer release becomes usable after a ViaLite
release lands and you restart Gate.
