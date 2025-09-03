---
title: "Install Gate with Go - Build from Source"
description: "Install Gate Minecraft proxy using Go compiler. Build from source with latest features and custom configurations."
---

# Install using Go

If you have [Go](https://go.dev/doc/install)
and [Git](https://www.atlassian.com/git/tutorials/install-git)
you can install Gate using the `go install` command:

```sh console
go install go.minekube.com/gate@latest
```

## Go Run

Using Go you could even run Gate without the installation command.
Internally Go downloads and caches the module, builds and caches the compiled object files
and finally runs it, all at once.

```sh console
go run go.minekube.com/gate@latest
```

Thanks to local caching running this command a second time would start Gate even quicker
since all the required dependencies are already on the local machine.

## Developers Guide

If you are a developer checkout the [Developers Guide](/developers/)!
