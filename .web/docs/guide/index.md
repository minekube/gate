
# Introduction

_Gate is a modern cloud-native, open source, fast, batteries-included and secure proxy for Minecraft servers
with a focus on scalability, flexibility, multi-version support and developer friendliness._

---

![server list ping](/images/server-list.png)

<!--@include: ../badges.md -->

Gate is a tiny [binary](install/binaries) and can run locally, in [Docker](install/docker) containers or
scale with your growing demands in a [Kubernetes](install/kubernetes)-orchestrated
production environment in the cloud.

It replaces legacy proxies like BungeeCord but also runs alongside them.
Gate is entirely written in Go and heavily inspired by the Velocity project.

::: tip What is Go?
Gate is written in [Go](https://go.dev/),
an easy-to-learn, fast, reliable, efficient, statically typed, compiled programming language designed at Google.
It is one of the most used languages for modern applications and one of the fastest growing programming languages
that is used by companies like Google, Microsoft, Meta, Amazon, Twitter, PayPal, Twitch, Netflix, Dropbox, Uber, Cloudflare, Docker, and many more.
:::

## Quick Start

If you already know the concepts of a Minecraft proxy,
you can skip this page and jump to the [Quick Start](quick-start) guide.

If you are a developer checkout the [Developers Guide](/developers/).

## Why do we need a Minecraft proxy?

### Use-cases

* You want to keep players connected to the proxy to move them between your different game servers like they would change the world.
* You want to enable cross game server plugins that e.g. handle player chat events or register proxy-wide commands
  broadcast messages and more.
* You want to intercept and log packets on the network traffic between players and servers

### How does a Minecraft proxy work?

Gate presents itself as a normal Minecraft server in the player's server list,
but once the player connects Gate forwards the connection to one of the actual
game servers (e.g. Minecraft vanilla, paper, spigot, sponge, etc.) to play the game.

The player can be moved around the network of Minecraft servers **without**
fully disconnecting, since we want the player to stay connected (and not want
them to re-login via the server-list every time).

Therefore, Gate reads all packets sent between players (Minecraft client) and
upstream servers, logs session state changes, emits different events like
[Login, Disconnect, ServerConnect, Chat, Kick etc.](https://github.com/minekube/gate/blob/master/pkg/edition/java/proxy/events.go)
that custom plugins/code can react to.

The **advantages** for using a proxy are far-reaching depending on your use-case.
