# Developers Guide

Gate is build with extensibility in mind, let's see how you can extend Gate with your code!

Throughout the docs and code comments we refer to your custom code as a plugin, extension or module.

::: details Note on Go's plugin system

We don't support Go's plugin system as it is not a mature solution. They force your plugin implementation to be
highly-coupled with Gate's build toolchain, the end-result would be very brittle, hard to maintain, and the overhead
would be much higher since the plugin author does not have any control over new versions of Gate.

One better solution would be to publish Go modules as Gate extensions where users can install
open source plugins from GitHub like: `gate mod install`
As implemented in [Hugo Modules](https://gohugo.io/hugo-modules/use-modules/)

:::
