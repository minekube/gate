# Quick Start

_This page quickly explains how to run Gate as a Minecraft proxy for your servers._

---

<!--@include: install/index.md -->

## Running Gate

After installing the binary, you can run the Gate Minecraft proxy using the `gate` command.

```sh console
$ gate
INFO	gate/root.go:93	logging verbosity	{"verbosity": 0}
INFO	gate/root.go:94	using config file	{"config": ""}
INFO	config	gate/gate.go:205	config validation warn	{"warn": "java: No backend servers configured."}
INFO	java	proxy/proxy.go:299	Using favicon from data uri	{"length": 3086}
INFO	java	proxy/proxy.go:472	listening for connections	{"addr": "0.0.0.0:25565"}

```

## Configuring Backend Servers

Gate connects to your Minecraft servers and forwards client connections to them.

You can do this by creating and editing the `config.yml` file.

```yaml config.yml
<!--@include: ../../../config-simple.yml -->
```

The `servers` section defines the addresses of your Minecraft servers.
and the `try` section defines the order in which players fallback to connect to.

There are many more options to configure, see [Configuration](/guide/config/) for more!
