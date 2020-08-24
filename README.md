![Logo](site/static/images/cover3.png)

# The Minecraft Proxy _(alpha)_

[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/minekube/gate?sort=semver)](https://github.com/minekube/gate/releases)
[![Doc](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go)](https://pkg.go.dev/go.minekube.com/gate)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/minekube/gate?logo=go)](https://golang.org/doc/devel/release.html)
[![Go Report Card](https://goreportcard.com/badge/go.minekube.com/gate)](https://goreportcard.com/report/go.minekube.com/gate)
[![test](https://github.com/minekube/gate/workflows/test/badge.svg)](https://github.com/minekube/gate/actions?query=workflow%3Atest)
[![Discord](https://img.shields.io/discord/633708750032863232?logo=discord)](https://discord.gg/6vMDqWE)

**Gate is an extensible Minecraft proxy written in Go**

> This project is not yet ready for production and
> subject to have breaking changes,
> but you can already start playing around with it!
>
> It is already being used by the Minekube network!

### Features

- [**Fast**](#benchmarks)
- Excellent server version support
    - Newest version down to 1.7 (+forge support)
    - Bungeecord plugins compatible (plugin messages)
    - Velocity's player forwarding mode
- [Quick installation](#quick-start)
    - simply pick a download from the [releases](https://github.com/minekube/gate/releases)
    - or quickly run our [docker image](https://gitlab.com/minekube/gate/container_registry)!
        - `docker run -it --rm -p 25565:25565 registry.gitlab.com/minekube/gate:latest`
    - (No Java runtime needed for Gate itself)
- A simple API to [extend Gate](#extending-gate-with-custom-code)
- Built-in [rate limiter](#rate-limiter)
- Automatic server icon resizing to 64x64
- Benefits from Go's awesome language features
    - simple, reliable, efficient
    - [and much more](https://golang.org/)

### Target audiences
- advanced networks wanting performance while operating at a high scale
- simple Minecraft network admins when [scripting languages](#script-languages)
are supported

![Server list](site/static/images/server-list.png)

## What Gate does

The whole point of a Minecraft proxy is to be able to
move players between servers without fully disconnecting them,
like switching the world but server-wise.

Similar to the proxies
[Velocity](https://github.com/VelocityPowered/Velocity)
_(where much of the knowledge and ideas for this proxy comes from)_,
[BungeeCord](https://github.com/SpigotMC/BungeeCord),
[Waterfall](https://github.com/PaperMC/Waterfall) etc.
Gate delivers rich interfaces to interact with connected players
on a network of Minecraft servers.

Therefore, Gate reads all packets sent between
players (Minecraft client) and servers (e.g. Minecraft spigot, paper, sponge, ...),
logs state changes and emits different events that 
custom plugins/code can react to.

## Why use Gate instead of one of the Java proxies?

An always asked question: "Whats the difference between Gate and
BungeeCord, for instance?"

First off, if you have all your code base in Java, just stay there.
Gate is for the Go ecosystem and does not have all the convenient
plugins published on SpigotMC!

Although Gate targets better performance, has more version support
(modded servers) and simpler API than known proxies like BungeeCord
and Gate lets you write server code in Go <3.

## Rate Limiter

Rate limiting is an important mechanism for controlling
resource utilization and managing quality of service.

Defaults set should never affect legitimate operations,
but rate limit aggressive behaviours.

In the `quota` section you can configure rate limiter
to block too many connections from the same IP-block (255.255.255.xxx).
    
**Note:** _The limiter only prevents attacks on a per IP block bases
and cannot mitigate against distributed denial of services (DDoS), since this type
of attack should be handled on a higher networking layer._
    
## Benchmarks

Gate has already been tested to successfully handle thousands of incoming connections
(even with rate limiter disabled).

> TODO: PRs are always welcome...
> Proper benchmarks will be added when Gate is stable enough.

## Quick Start

This is an example Minecraft network using Gate proxy,
a Paper 1.16.1 (server1) & Paper 1.8.8 (server2).

_You will need Java Runtime 8 or higher for running the Paper servers._

1. `git clone https://github.com/minekube/gate.git`
2. Run `server1` and `server2` found in `docs/sample/`
    - `java -jar <server>.jar`
3. Run Gate within `docs/sample` to use sample config
    - [Download a release here](https://github.com/minekube/gate/releases)
    - Or compile it yourself: `go build .`
    - Run `gate` (help flag `-h`)
    
Now you can connect to the network on `localhost:25565`
with a Minecraft version 1.16.1 and 1.8.x.
Gate tries to connect you to one of the servers as specified in the configuration.

> There will be an expressive documentation website for Gate later on!

## Extending Gate with custom code

- [Native Go](#native-go)
- [Script languages](#script-languages)

### Native Go

You can import Gate as a Go module and use it like a framework
in your own project.

Go get it into your project:
```
go get -u go.minekube.com/gate
```

**Refer to [plugin.go](https://github.com/minekube/gate/blob/master/pkg/proxy/plugin.go)
for detailed information!**

> TODO: code examples

### Script languages

To simplify and accelerate customization of Gate there
will be added support for scripting languages such as
[Tengo](https://github.com/d5/tengo) and
[Lua](https://github.com/yuin/gopher-lua).

> This feature will be added when highly requested.

## Anticipated future of Gate

- Gate will be a high performance & cloud native Minecraft proxy
ready for massive scale in the cloud!

- Players can always join and will never be kicked if there is
no available server to connect to, or the network is too full.
Instead, players will be moved to an empty virtual room simulated
by the proxy to queue players to wait.

- _Distant future, or maybe not too far?_ A proxy for Java & Bedrock edition to mix and match players & servers of all kinds.
(protocol translation back and forth...)
