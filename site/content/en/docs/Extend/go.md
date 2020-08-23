---
title: "Extending Gate with native Go code"
linkTitle: "Native Go"
weight: 1
description: >
  This guide covers how you can use Gate in your Go projects.
---

While Gate can be run as a binary, you can also import and use it like a _framework_ in your Go projects
and add custom functionality!

For the purpose of this guide we're going to explain the
{{< ghlink href="examples/extend/simple-proxy/" >}}simple-proxy{{< /ghlink >}}
example in more details.

## Creating a new project

1. Create a new project (e.g. `simple-proxy`)
2. `go mod init simple-proxy`
3. Get Gate: `go get -u go.minekube.com/gate`

### Plug-in

In this example where begin to embed Gate in the main function, but it can be done anywhere you need it.

```go
package main

func main() {
	// Add our "plug-in" to be initialized on Gate start.
	proxy.Plugins = append(proxy.Plugins, proxy.Plugin{
		Name: "SimpleProxy",
		Init: func(proxy *proxy.Proxy) error {
			return newSimpleProxy(proxy).init()
		},
	})

	// Execute Gate entrypoint and block until shutdown.
	// We could also run gate.Run if we don't need Gate's command-line.
	gate.Execute()
}
```

Refer to **{{< ghlink href="examples/extend/simple-proxy/" >}}simple-proxy{{< /ghlink >}}** to see all code!