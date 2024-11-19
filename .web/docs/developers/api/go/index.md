# <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/go/go-original-wordmark.svg" class="tech-icon" alt="Go" /> Go Client

Gate offers two powerful approaches to integrate with Go applications:

<!--@include: ./integration-options.md -->

If you choose to use the HTTP API Client, follow along below.

## HTTP API Client

1. Install the required packages:

```bash
go get buf.build/gen/go/minekube/gate/connectrpc/go@latest
go get buf.build/gen/go/minekube/gate/protocolbuffers/go@latest
```

2. Example usage:

```go
<!--@include: ./main.go -->
```

3. Run the example:

```bash
go run .
{
  "servers": [
    {
      "name": "server2",
      "address": "localhost:25567"
    },
    {
      "name": "server3",
      "address": "localhost:25568"
    },
    {
      "name": "server4",
      "address": "localhost:25569"
    },
    {
      "name": "server1",
      "address": "localhost:25566"
    }
  ]
}
```

This example project is located in the [`docs/developers/api/go`](https://github.com/minekube/gate/tree/main/.web/docs/developers/api/go) directory.

::: info Learn More
For more details on using ConnectRPC with Go, check out the [ConnectRPC Documentation](https://connectrpc.com/docs/go/getting-started#make-requests).
:::

<style>
.tech-icon {
  width: 32px;
  height: 32px;
  display: inline-block;
  vertical-align: middle;
  margin-right: 12px;
  position: relative;
  top: -2px;
}
</style>
