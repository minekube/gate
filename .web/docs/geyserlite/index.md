---
title: "GeyserLite - Managed Bedrock Runtime for Gate"
description: "GeyserLite packages GeyserMC as native artifacts and embeddable libraries so Gate can manage Bedrock cross-play with low overhead."
---

# GeyserLite

GeyserLite is Minekube's native runtime shape for
[GeyserMC](https://geysermc.org/). Gate uses it to provide managed Bedrock
cross-play without asking operators to run a separate JVM process.

For most Gate users, this is the only config needed:

```yaml
config:
  bedrock: true
```

Gate starts the managed Bedrock runtime, generates the Floodgate key material,
and connects GeyserLite to Gate's Java listener.

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
events, forwarding, and backend login.

If ViaLite is also enabled, it sits behind Gate. That means Bedrock traffic is
already translated to Java before ViaLite handles backend version translation.

## Managed Config

Use the shorthand for the default managed setup:

```yaml
config:
  bedrock: true
```

Use object form when you need to customize the listener, username format, or
Geyser config overrides:

```yaml
config:
  bedrock:
    enabled: true
    usernameFormat: ".%s"
    geyserListenAddr: "localhost:25567"
    managed:
      enabled: true
      configOverrides:
        bedrock:
          port: 19132
```

Gate keeps the Java-side listener private by default. Expose
`geyserListenAddr` only when GeyserLite runs outside the same network namespace.

## Version Policy

GeyserLite tracks upstream Geyser through a pinned source ref and a managed
release chain. When a GeyserLite release is published, Gate can consume it as a
managed dependency.

Production guidance follows upstream Geyser support:

- If Geyser officially supports the relevant Java and Bedrock versions, Gate's
  managed Bedrock mode is the stable path.
- If a server updates to a brand-new Java version before Geyser supports it,
  operators should usually wait before upgrading production backends.
- ViaLite may help early adopters when only the Java backend protocol is newer,
  but it cannot fix unsupported Bedrock client protocols or missing Geyser
  Bedrock-to-Java translation.

## Runtime Modes

GeyserLite ships as:

- Native executable for managed subprocess mode.
- Shared library for embedded Go and Rust integrations.
- Container image for standalone deployments.

Gate chooses the managed runtime shape supported by the current platform and
configuration.

## Links

- [Gate Bedrock guide](/guide/bedrock)
- [GeyserLite repository](https://github.com/minekube/geyserlite)
- [GeyserLite releases](https://github.com/minekube/geyserlite/releases)
- [GeyserMC supported versions](https://geysermc.org/wiki/geyser/supported-versions/)
