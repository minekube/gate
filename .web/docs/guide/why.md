# Why

_We recommend reading the [Introduction](index) if you haven't already._

Let's start with where we come from, the problem definition,
different parties involved and the solution space.

## Advantages

- Fast and needs less resources (CPU/Memory) leading to improved scalability
- Excellent protocol version support
  - Allows newest version down to 1.7
  - Forge support (for modded servers)
  - BungeeCord compatible plugin channels
  - BungeeCord or Velocity's player info forwarding
- A simple API for plugins/extensions
  - Extend with [your Go code](https://github.com/minekube/gate/tree/master/.examples/extend/simple-proxy)
  - Or use a [script language](https://github.com/minekube/gate/issues/9)
- Perfect for Go developers - Gate and developers immensely benefit from the Go language and its wide ecosystem

Similar to the Minecraft proxies written in Java: BungeeCord, Waterfall and Velocity
_(where much of the knowledge comes from)_
Gate delivers a rich interface to interact with connected players
on your cluster of Minecraft servers.


## Target audience

Gate supports small and large Minecraft networks that need to serve thousands of
concurrent players and encourages new and established Golang developers to extend Gate.

Although Gate targets better performance, has more version support
(modded servers) and has a simpler API than the BungeeCord Java proxy,
and Gate lets you write extension code in the awesome Go programming language...

_If you already have all your code base in Java or need to
use plugins for other proxies like BungeeCord and Velocity, just stay there._
You can't use Java plugins from SpigotMC with Gate.

## Why not use an existing proxy written in Java?

Because the less Java a smart Go developer needs to maintain, the happier the Go community.
Since Go developers work in a very fast-paced and cloud-centric ecosystem with a lot of modern
software written in Go there is simply no cognitive space and time left for [Java, where
everything is bigger](https://youtu.be/PAAkCSZUG1c?t=317).

_The ONLY Java code you must write is for Paper/Spigot/Minestom plugins,
since there is no Go Minecraft server implementation as
nobody can keep up-to-date quick enough with Mojang's releases
of new vanilla server versions that break the protocol everytime._ - unless you are using
[Skript](https://forums.skunity.com/resources/skript.323/) ;), it's awesome for beginners
and has no limits for advanced use-cases.
