[![Logo](.web/docs/public/og-image.png)](https://gate.minekube.com)

# The extensible Minecraft Proxy

[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/minekube/gate?sort=semver)](https://github.com/minekube/gate/releases)
[![Doc](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go)](https://pkg.go.dev/go.minekube.com/gate)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/minekube/gate?logo=go)](https://golang.org/doc/devel/release.html)
[![Go Report Card](https://goreportcard.com/badge/go.minekube.com/gate)](https://goreportcard.com/report/go.minekube.com/gate)
[![test](https://github.com/minekube/gate/workflows/ci/badge.svg)](https://github.com/minekube/gate/actions)
[![Discord](https://img.shields.io/discord/633708750032863232?logo=discord)](https://discord.gg/6vMDqWE)

**Gate is an extensible, high performant & paralleled
Minecraft proxy** server with scalability, flexibility &
excellent server version support -
_written in Go and ready for the cloud!_

> Gate is currently subject to have breaking changes,
> but you can already start using it!
> It is already being used by our wide [community](https://minekube.com/discord) and powers the open [Connect Network](https://connect.minekube.com/)!

## [Website & Documentation](https://gate.minekube.com)

**There is a lot to discover on Gate's website.**
Please refer to the website for the [documentation](https://gate.minekube.com),
guides and any more information needed!

## Quick Start

Follow our [quick start guide](https://gate.minekube.com/guide/quick-start/) on creating a simple Minecraft network!

[![Server list](/images/server-list.png)](https://gate.minekube.com)


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
```
