---
title: 'Gate Simple Proxy Example - Getting Started'
description: 'Build your first Gate Minecraft proxy extension. Complete example showing how to create custom functionality with Go programming and Gate APIs.'
---

# Simple Proxy • Example

`Simple Proxy` is a runnable example Go project that showcases all the basic features of Gate.
You can use this project as a template for your own Gate plugins.

## Running the Simple Proxy

```sh console
git clone https://github.com/minekube/gate.git
cd gate/.examples/extend/simple-proxy
go run .
```

## Run any Minecraft backend server

You must run a backend server to actually join the game and see Simple Proxy in action.
Open a new terminal and start a Minecraft server:

```sh console
cd .examples/simple-network/server1
java -jar *.jar -nogui
```

Join the proxy at `localhost:25565`.

## Learning Task

> If you change code in the Simple Proxy project, you must restart the proxy to see the changes.
> (press `CTRL+C` to stop the proxy)

Add a `/playerinfo` command to the proxy that prints the player details like username and play duration.

::: details Solution
```go
<!--@include: result_task1.go -->
````
:::

## Code

You can check out the project on [GitHub](https://github.com/minekube/gate/tree/master/.examples/extend/simple-proxy).

```go
<!--@include: ../../../../.examples/extend/simple-proxy/proxy.go -->
```
