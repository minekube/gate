---
title: "Quickstart: Run the Gate binary"
linkTitle: "Running Gate"
weight: 20
description: >
  This guide covers how you can quickly get started using Gate
  to create a sample Minecraft network to play on!
---

## Prerequisites

The following prerequisites are required to create a Minecraft network:

- Install a **Gate binary** with [these instructions]({{< ref "/docs/Installation/_index.md" >}})
- **Java**: You need a Java Runtime Environment (JRE) version 8 or higher to run the Minecraft servers

For the purpose of this guide we're going to use the
{{< ghlink href="examples/simple-network/" >}}simple-network{{< /ghlink >}}
example as a pre-configured Minecraft network.

## Objectives
- Download the sample network
- See configuration
- Run Gate proxy
- Run Minecraft servers

## Steps

### 1. Download

[Download Gate as a zip]({{< param zip >}}) and extract the `examples/simple-network` directory.

### 2. Configuration

Being inside the `examples/simple-network` folder you see a `config.yaml` for Gate configuration.
```yaml
# This is a simplified config where the rest of the
# settings are omitted and will be set by default.
bind: 0.0.0.0:25565
onlineMode: true
servers:
  server1: localhost:25566
  server2: localhost:25567
try:
  - server1
  - server2
```
Gate will listen for connection on all IPs on the host at port 25565 (default port) and
register two servers that players can be redirected to.

Looking inside the server folders we:
- set the `server-port`, `online-mode=false` and `bungeecord=true`
- accept [Mojang's EULA](https://account.mojang.com/documents/minecraft_eula)

{{< alert title="Note" color="info">}}
We disabled the nether and end world for faster startup times.
{{< /alert >}}

### 3. Running Gate

Make sure you are have a terminal open inside the same path as the `config.yaml` and run the Gate binary with:
- Windows: `gate.exe`
- Linux: `./gate`

(Append the `-h` flag to display help.)

### 4. Start Minecraft servers

In the same directory you find a `server1` and `server2` _(folder names are not of importance)_.

Open a terminal for each server inside the appropriate directory and start the server jar:
-  Run: `java -jar <server.jar>`

The first start will generate all other files.

**Now you can join `localhost:25565` with the appropriate Minecraft client version and use the
builtin `/server` command to switch your server!**