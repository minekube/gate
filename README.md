[![Logo](.web/docs/public/og-image.png)](https://gate.minekube.com)

# The extensible Minecraft Proxy

[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/minekube/gate?sort=semver)](https://github.com/minekube/gate/releases)
[![Doc](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go)](https://pkg.go.dev/go.minekube.com/gate)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/minekube/gate?logo=go)](https://golang.org/doc/devel/release.html)
[![Go Report Card](https://goreportcard.com/badge/go.minekube.com/gate)](https://goreportcard.com/report/go.minekube.com/gate)
[![test](https://github.com/minekube/gate/workflows/ci/badge.svg)](https://github.com/minekube/gate/actions)
[![Discord](https://img.shields.io/discord/633708750032863232?logo=discord)](https://discord.gg/6vMDqWE)

**Gate is an extensible, high performant & paralleled
Minecraft proxy** server with scalability, flexibility,
cross-platform compatibility & excellent server version support -
_written in Go and ready for the cloud!_

## [Website & Documentation](https://gate.minekube.com)

**There is a lot to discover on Gate's website.**
Please refer to the website for the [documentation](https://gate.minekube.com),
guides and any more information needed!

## Quick Start

Follow our [quick start guide](https://gate.minekube.com/guide/quick-start/) on creating a simple Minecraft network!

| Platform    | Installation Command                                               |
| ----------- | ------------------------------------------------------------------ |
| Go          | `go run go.minekube.com/gate@latest`                               |
| Linux/macOS | `curl -fsSL https://gate.minekube.com/install \| bash`             |
| Windows     | `powershell -c "irm https://gate.minekube.com/install.ps1 \| iex"` |

[![Server list](.web/docs/images/server-list.png)](https://gate.minekube.com)

## Bedrock Cross-Play Support

Gate includes built-in **Bedrock Edition support** enabling cross-play between
Java Edition (PC) and Bedrock Edition (Mobile, Console, Windows 10) players
through integrated Geyser & Floodgate technology - **zero plugins required**!

See the [Bedrock Guide](https://gate.minekube.com/guide/bedrock/) for setup instructions.

```mermaid
graph LR
    A[Java Players<br/>PC] -->|25565| D(Gate Proxy)
    B[Bedrock Players<br/>Mobile/Console/Win10] -->|19132| C(Geyser)
    C -->|25567| D
    D -->|Java Protocol| E[Backend Server<br/>Paper/Spigot/Vanilla]

    style A fill:#b36b00,stroke:#333,stroke-width:2px
    style B fill:#007a7a,stroke:#222,stroke-width:2px
    style C fill:#1e90ff,stroke:#222,stroke-width:2px
    style D fill:#2e8b57,stroke:#222,stroke-width:2px
    style E fill:#a0526d,stroke:#222,stroke-width:2px
```

## Gate Lite Mode

Gate has a Lite Mode which is a lightweight version of Gate that can expose
multiple Minecraft servers through a single port and IP address and reverse proxy
players to backend servers based on the hostname/subdomain they join with.

See the [Lite Mode](https://gate.minekube.com/guide/lite/) guide for more information.

```mermaid
graph LR
    A[Player Alice] -->|Join example.com| C(Gate Lite)
    B[Player Bob] -->|Join my.example.com| C(Gate Lite)
    C -->|10.0.0.1| D[Backend A]
    C -->|10.0.0.2| E[Backend B]
    C -->|10.0.0.3| F[Another Proxy]

    linkStyle 0 stroke:orange
    linkStyle 1 stroke:purple
    linkStyle 2 stroke:purple
    linkStyle 3 stroke:orange
```

## Developers Starter Template

The starter template is designed to help you get started with your own Gate powered project.
Fork it! 🚀 - [minekube/gate-plugin-template](https://github.com/minekube/gate-plugin-template)
