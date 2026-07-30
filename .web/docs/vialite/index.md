---
title: "ViaLite - Managed Java Version Compatibility for Gate"
description: "ViaLite gives Gate managed Via-powered backend protocol translation for Java clients and Bedrock players already translated through GeyserLite."
---

# ViaLite

ViaLite is Minekube's managed Via runtime for Gate classic. It lets Gate route
backend connections through Via-powered Java protocol translation while Gate
keeps ownership of authentication, events, routing, Connect, and backend login.

For user-facing setup, start with [Multi-Version Support](/guide/multi-version).
This page covers the runtime shape behind that feature.

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

ViaLite sits behind Gate, not in front of it. Gate has already accepted the
player and selected a backend before ViaLite translates backend packets.

## Not Lite Mode

Gate Lite intentionally reads the initial handshake, chooses a backend, and
then raw-pipes bytes so backend servers keep authentication ownership. ViaLite
must decode and rewrite packets after login, so it belongs to Gate classic
backend connections instead.

## Runtime Configuration

By default, Gate uses the latest stable ViaLite release for managed
multi-version support. Operators can still pin or override the runtime when
they need controlled rollout or offline deployment.

Subprocess mode is the portable default. Embedded mode is available where the
native shared library is supported. See
[Multi-Version Support](/guide/multi-version) for the basic enablement config.

## Early Backend Upgrades

ViaLite can bridge some early-upgrade scenarios:

- A Java backend moves to a newer Minecraft server version.
- Gate can still accept the client session.
- Via supports translation between Gate's backend-facing protocol and that
  backend version.

For Bedrock players, GeyserLite must still be able to translate the Bedrock
session into Java and connect to Gate. ViaLite can help with the Java backend
side after that point, but it cannot add missing Geyser Bedrock protocol
support.

## Update Chain

ViaLite releases publish checksummed native artifacts. Gate consumes those
releases as a managed dependency, so the normal chain is:

```text
ViaLite release
  -> Gate managed dependency bump
  -> Gate release
  -> downstream deployments
```

Use pins only for controlled rollouts. Otherwise let Gate's managed runtime use
the stable ViaLite release channel.

## Links

- [Multi-Version Support](/guide/multi-version)
- [Gate compatibility guide](/guide/compatibility)
- [GeyserLite docs](/geyserlite/)
- [ViaLite repository](https://github.com/minekube/vialite)
- [ViaLite releases](https://github.com/minekube/vialite/releases)
- [ViaVersion](https://viaversion.com/)
