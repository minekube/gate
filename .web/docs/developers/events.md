---
title: 'Gate Minecraft Proxy Events System'
description: 'Learn about Gate's event system for Minecraft proxy development. Handle player events, server connections, chat messages, and custom event listeners.'
---

# Events

_Gate provides a powerful event system that allows you to listen to and modify events that occur in the proxy._

## Subscribing to Events

Events are a way to communicate between different parts between your code and Gate.
They are a way to decouple Gate from your own application code and make it more flexible.

Checkout the [Simple Proxy](examples/simple-proxy#code) for more examples.

Example:
```go
<!--@include: subscribe_example.go -->
```


## Available Events

::: details Available Events

See source on [GitHub](https://github.com/minekube/gate/blob/master/pkg/edition/java/proxy/events.go).

```go
<!--@include: ../../../pkg/edition/java/proxy/events.go -->
```
:::
