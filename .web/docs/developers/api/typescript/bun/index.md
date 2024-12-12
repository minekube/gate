# <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/bun/bun-original.svg" class="tech-icon" alt="Bun" /> Bun

You can use the following `bunfig.toml` to install the dependencies from the `buf.build` registry.

::: code-group

```toml [bunfig.toml]
<!--@include: ./bunfig.toml -->
```

:::

To install dependencies:

::: code-group

```bash [bun]
bun add @buf/minekube_gate.connectrpc_es@latest
```

```bash [pnpm]
pnpm add @buf/minekube_gate.connectrpc_es@latest
```

```bash [npm]
npm install @buf/minekube_gate.connectrpc_es@latest
```

```bash [yarn]
yarn add @buf/minekube_gate.connectrpc_es@latest
```

:::

Refer to the [ConnectRPC](https://connectrpc.com/docs/node/using-clients) documentation for more information on how to use ConnectRPC with TypeScript on server side.

::: tip Browser support

To use the Gate API in the browser, check out the [Web](/developers/api/typescript/web/) documentation.

:::

::: code-group

```ts [index.ts]
<!--@include: ./index.ts -->
```

:::

## Sample project

This sample project is located in the [`docs/developers/api/typescript/bun`](https://github.com/minekube/gate/tree/master/.web/docs/developers/api/typescript/bun) directory.

To install dependencies:

```bash
bun install
```

To run:

```bash
bun run index.ts
[
  {
    "name": "server1",
    "address": "localhost:25566",
    "players": 0
  },
  {
    "name": "server2",
    "address": "localhost:25567",
    "players": 0
  },
  {
    "name": "server3",
    "address": "localhost:25568",
    "players": 0
  },
  {
    "name": "server4",
    "address": "localhost:25569",
    "players": 0
  }
]
```

This project was created using `bun init` in bun v1.1.26. [Bun](https://bun.sh) is a fast all-in-one JavaScript runtime.

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
