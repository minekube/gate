---
title: "Overview"
linkTitle: "Overview"
weight: 1
description: >
  Gate is an extensible Minecraft proxy written in Go and can be
  run as a binary or be integrated like a _framework_ with your own code.
---

## What is Gate?

Gate is an extensible, high performant & paralleled Minecraft proxy server
with scalability, flexibility & excellent server version support - _ready for the cloud_!

{{< alert title="Note" color="info">}}
Gate is written in [Go (simple, reliable, efficient)](https://golang.org/),
a statically typed, compiled programming language
designed at Google and is syntactically similar to C, but with memory safety,
garbage collection, structural typing, and CSP-style concurrency.
{{< /alert >}}


### Why you need a Minecraft proxy?
*(for those who have never heard of a Minecraft proxy)*

**TL;DR**
* keep the player's session without disconnect
* to move players between servers
* cross server functionalities (events such as chat/commands, send messages,
...builtin/custom session- & packet handlers)

Gate presents itself as a normal Minecraft server in the player's server list,
but once the player connects Gate forwards the connection to one of the actual
game servers (e.g. Minecraft vanilla, paper, spigot, sponge, etc.) to play the game.

The player can be moved around the network of Minecraft servers **without**
fully disconnecting, since we want the player to stay connected (and not want
them to re-login via the server-list every time).

Therefore, Gate reads all packets sent between players (Minecraft client) and
upstream servers, logs session state changes, emits different events like
[Login, Disconnect, ServerConnect, Chat, Kick etc.](https://github.com/minekube/gate/blob/master/pkg/proxy/events.go)
that custom plugins/code can react to.

The **advantages** for using a proxy should be clear now.



## Why Gate?

**Some of Gate's advantages:**

- Fast and less resources (CPU/Memory) needed
    - means more scalable
- Excellent version support
    - Allows newest version down to 1.7
    - Forge support (for modded servers)
    - BungeeCord compatible plugin channels
    - BungeeCord or Velocity's player info forwarding
- A much simpler API for "plugins"
    - Extend with [your Go code]({{< ref "/docs/Extend/go.md" >}})
    - Or use a [script language]({{< ref "/docs/Extend/scripts.md" >}})
- Gate and developers immensely benefit from the Go language and its wide ecosystem

Similar to the in Java written proxies BungeeCord, Waterfall and Velocity
_(where much of the knowledge comes from)_
Gate delivers a rich interface to interact with connected players
on your cluster of Minecraft servers.

{{< alert title="Before you may ask:" color="info">}}
_"Why not use an existing proxy written in Java?"_

Because the less Java we need to maintain, the happier we are at
[Minekube](https://minkeube.net/discord), since we need and work in a very
fast paced and cloud-centric ecosystem with a lot of Kubernetes & controllers,
Protobuf & GRPC, CockroachDB, Agones, Istio, Nats and so forth,
there is no space and time for Java.

_(The ONLY Java code we must write is for paper/spigot plugins,
since there is no Go Minecraft server implementation because
no one can keep up to date fast enough with Mojang's quick releases
of new vanilla server features.)_
{{< /alert >}}

### Target audience
Note that Gates targets advanced Minecraft networks
**that already (or want to) have a Go code base**
for their Minecraft related workloads.

_If you already have all your code base in Java and/or need to
use plugins for those other proxies, just stay there._
Gate is for the Go ecosystem and does not have all the convenient
plugins published on SpigotMC!

Although Gate targets better performance, has more version support
(modded servers) and has a simpler API than other Java proxies,
and Gate lets you write "plugin" code in the awesome Go programming language <3.


## Major Features

While Gate has all the features you expect a modern Minecraft proxy to have,
Gate incorporates these additional abilities:

- Extensibility!
- All expected modern Minecraft proxy capabilities
- Simple configuration
    - Sane defaults!
    - Use a config file and/or environment variables to override defaults
- Simple deployment
    - Single binary for all major platforms!
    - Ready to use Docker image
- Builtin IP range [rate limiter]({{< ref "/docs/Configuration/_index.md" >}}) (anti-DoS)
    - Configurable in config
- Automatic server icon resizing to 64x64
    - Resizes your server list icon down to the maximum allowed size

# What Next?
- Have a look at our [installation guides]({{< ref "/docs/Installation/_index.md" >}}), for installing Gate.
- Go through our [Quickstart Guides]({{< ref "/docs/Getting Started/_index.md" >}}) to take you through setting
up a simple Minecraft network using Gate.