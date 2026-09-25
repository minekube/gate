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

## Which Runtime Runs

`config.via.version` decides which ViaLite release the proxy actually runs:

```yaml
config:
  via:
    enabled: true           # runtime resolution: newest stable ViaLite release
    # version: v0.3.1       # pin an exact release for a controlled rollout
    # binaryPath: /usr/local/bin/vialite   # use a local binary, no download
    # libraryPath: /usr/local/lib/libvialite.so
    # offline: true         # never download
    # mirror: https://mirror.example.com/vialite
```

- **Default (`version` unset, `auto`, or `latest`)** resolves the newest stable
  ViaLite release **at startup**. A newly published runtime reaches an operator
  on their next Gate restart: there is no hot swap, and the runtime is fetched
  per start rather than bundled with Gate. So a restart picks up new Minecraft
  version support without upgrading Gate.
- **Pinning** with an exact tag (`version: v0.3.1`) freezes the runtime until
  the pin changes. Use it for controlled rollouts; remember that the pinned
  runtime also freezes the newer-client support it carries.
- **Local artifacts** (`binaryPath`, `libraryPath`) or `offline: true` bypass
  downloads entirely for air-gapped or pre-seeded deployments.
- This is **independent of the Go module version Gate links**: the artifact is
  resolved and downloaded separately from Gate's own build.

Every start logs the resolved runtime once, so the live build is never a guess:

```text
INFO msg="vialite: resolved runtime" kind=binary source=download version=v0.3.1 path=/home/gate/.cache/vialite/v0.3.1/<sha256>/vialite-linux-amd64 url=https://github.com/minekube/vialite/releases/download/v0.3.1/vialite-linux-amd64
INFO msg="vialite: resolved runtime" kind=binary source=cache version=v0.3.1 path=...
INFO msg="vialite: resolved runtime" kind=binary source=binaryPath path=/usr/local/bin/vialite
```

`source` is one of `download`, `cache`, `binaryPath`, `env:VIALITE_BINARY`,
`embedded`, or `path` (from `$PATH`); for libraries the corresponding
`libraryPath`, `env:VIALITE_LIBRARY`, `embedded`, and `system` sources.

### Download Cache

Downloaded runtimes are cached per release under the OS user cache directory,
keyed by release and artifact checksum
(`<cache>/vialite/<version>/<sha256>/<artifact>`, for example
`~/.cache/vialite/v0.3.1/<sha256>/vialite-linux-amd64`). A new release is a new
cache entry, so switching versions never serves a stale artifact; old entries
are left in place and can be deleted freely.

### Mirrors

`mirror` replaces the release download base (GitHub release URL layout). For an
unset/`auto`/`latest` version, Gate asks the mirror for its own latest release
first, so a mirror deployment follows new releases the same way GitHub does. A
mirror that only serves files and cannot report a latest release falls back to a
compiled-in release **and logs a warning naming it** - pin `version` explicitly
if you would rather not rely on that fallback. An explicit `latest` against a
mirror that cannot answer is an error rather than a silent downgrade.

### Why a New ViaVersion Needs a ViaLite Release

ViaVersion ships *inside* the ViaLite runtime artifact. A new ViaVersion
therefore reaches operators only after the upstream pin is refreshed, the native
artifact is rebuilt, and a new ViaLite release is published; a newer Minecraft
client joins a newer backend only once the runtime's bundled ViaVersion knows
that client's protocol. Gate itself needs to know the client's protocol too -
see [Multi-Version Support](/guide/multi-version).

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
