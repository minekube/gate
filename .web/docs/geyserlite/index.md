---
title: "GeyserLite - Managed Bedrock Runtime for Gate"
description: "GeyserLite is the managed runtime Gate uses to provide Bedrock cross-play through GeyserMC."
---

# GeyserLite

GeyserLite is Minekube's managed runtime packaging for
[GeyserMC](https://geysermc.org/). Gate uses it for Bedrock cross-play without
requiring operators to run and wire a separate Geyser process for the common
case.

For operator setup and configuration examples, use the
[Bedrock support guide](/guide/bedrock).

## Topology

```text
Bedrock player
  -> GeyserLite
  -> Gate classic
  -> optional ViaLite
  -> backend server
```

GeyserLite is the Bedrock ingress layer. It translates Bedrock protocol into
Java protocol before the connection reaches Gate. Gate then owns routing,
events, forwarding, Connect integration, and backend login.

If [ViaLite](/vialite/) is enabled, it sits behind Gate. Bedrock traffic is
already translated to Java before ViaLite handles backend version translation.

## Runtime Scope

GeyserLite is responsible for the managed Geyser runtime shape:

- Native executable for managed subprocess mode.
- Shared library for embedded integrations where supported.
- Container image for standalone deployments.
- Release artifacts and checksums that Gate can consume as a managed
  dependency.

Gate decides which runtime mode to use for the current platform and
configuration. The Bedrock guide covers the Gate config surface.

## Bedrock Transport and Client Currency

Gate's managed Bedrock runtime follows GeyserLite releases rather than a pinned
upstream Geyser build, so the Bedrock client line Gate accepts moves with the
GeyserLite version Gate pins:

- **Gate v0.76.2 pins GeyserLite v0.7.6** (`go.minekube.com/geyserlite` in
  Gate's `go.mod`).
- GeyserLite v0.7.x carries upstream Geyser stable **26.60** - the Bedrock
  **26.60** client line - and enables **native NetherNet ingress** on both Linux
  architectures. NetherNet adds a signalling transport next to the RakNet UDP
  port. A host that runs GeyserLite itself (standalone or subprocess) configures
  that address on the GeyserLite side (`GEYSER_SIGNALING_ADDRESS`, or
  `geyserSignalingAddress` for a subprocess host); it is not a Gate config key,
  and Gate's own managed runtime owns its listener.
- A managed Bedrock listener that stops answering at the **257th** open session,
  or logs `verified ingress queue overflow`, is older than **v0.75.10**, which
  drains the managed Geyser connection notifications instead of letting the
  queue accumulate.

## Version Policy

GeyserLite tracks upstream Geyser through a pinned source ref and a managed
release chain. Production guidance follows upstream Geyser support:

- If Geyser officially supports the relevant Java and Bedrock versions, Gate's
  managed Bedrock mode is the stable path.
- If a backend updates to a brand-new Java version before Geyser supports it,
  production networks should usually wait before upgrading Bedrock-critical
  servers.
- Preview pins are exceptional. They should require a concrete upstream Geyser
  preview build or PR and a clear user-impact reason.

ViaLite may help when only the Java backend protocol is newer, but it cannot
fix unsupported Bedrock client protocols or missing Geyser Bedrock-to-Java
translation.

## Links

- [Bedrock support guide](/guide/bedrock)
- [ViaLite](/vialite/)
- [GeyserLite repository](https://github.com/minekube/geyserlite)
- [GeyserLite releases](https://github.com/minekube/geyserlite/releases)
- [GeyserMC supported versions](https://geysermc.org/wiki/geyser/supported-versions/)
