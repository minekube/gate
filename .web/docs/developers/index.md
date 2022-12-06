# Developers Guide

_If you want to learn how to extend Gate with your own code, you are in the right place._

<!--@include: ../badges.md -->

## Getting Started

Gate is designed with developers in mind.

All you need to get started is a working Go environment. You can find the Go installation instructions [here](https://golang.org/doc/install).

Once you have Go installed, you create a new Go module and add Gate as a dependency:

```sh console
mkdir mcproxy; cd mcproxy
go mod init mcproxy
go get -u github.com/minekube/gate@latest
```

Add and initialize your plugin and execute Gate, that's it!

```go mcproxy.go
func main() {
    // Add our "plug-in" to be initialized on Gate start.
    proxy.Plugins = append(proxy.Plugins, proxy.Plugin{
        Name: "SimpleProxy",
        Init: func(ctx context.Context, proxy *proxy.Proxy) error {
            return newSimpleProxy(proxy).init() // see code examples
        },
    })
    
    // Execute Gate entrypoint and block until shutdown.
    // We could also run gate.Start if we don't need Gate's command-line.
    gate.Execute()
}
```

## Learning by Example

The best way to learn how to extend Gate is by looking at some examples.
If you want to see a complete Go project that uses Gate, check out the [Simple Proxy](examples/simple-proxy) example.

## Gate's Plugin Architecture

Gate does not follow a traditional plugin system like Java Bukkit or BungeeCord.
Instead, you create your own Go module, import Gate APIs, and compile it like a normal Go program.

Throughout the docs we refer to your custom code as a module, plugin or extension.

::: details Note on Go's plugin system

We don't support Go's plugin system as it is not a mature solution. They force your plugin implementation to be
highly-coupled with Gate's build toolchain, the end-result would be very brittle, hard to maintain, and the overhead
would be much higher since the plugin author does not have any control over new versions of Gate.

One better solution would be to publish Go modules as Gate extensions where users can install
open source plugins from GitHub like: `gate mod install`
As implemented in [Hugo Modules](https://gohugo.io/hugo-modules/use-modules/)

:::
