---
title: 'Gate Web API - Browser TypeScript for Minecraft'
description: 'Create web applications for Minecraft server management with Gate's browser TypeScript API. Frontend development for Minecraft proxy control.'
---
# <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/chrome/chrome-original.svg" class="tech-icon" alt="Web" /> Web

You can use the following `.npmrc` to install the dependencies from the `buf.build` registry.

::: code-group

```toml [.npmrc]
<!--@include: ./.npmrc -->
```

:::

or using `pnpm`:

```bash
pnpm config set @buf:registry https://buf.build/gen/npm/v1/
```

To install dependencies:

::: code-group

```bash [pnpm]
pnpm add @buf/minekube_gate.connectrpc_es@latest
```

```bash [bun]
bun add @buf/minekube_gate.connectrpc_es@latest
```

```bash [npm]
npm install @buf/minekube_gate.connectrpc_es@latest
```

```bash [yarn]
yarn add @buf/minekube_gate.connectrpc_es@latest
```

:::

Refer to the [ConnectRPC](https://connectrpc.com/docs/web/using-clients) documentation for more information on how to use ConnectRPC with TypeScript on browser side.

::: tip Browser support

To use the Gate API in the browser, check out the [Web](/developers/api/typescript/web/) documentation.

:::

::: code-group

```ts [index.js]
<!--@include: ./index.ts -->
```

:::

::: warning

Note that we had to append `.js` to the import path in line 3 due Node.js requiring `.js` for CommonJS modules, other than in [Bun](/developers/api/typescript/bun/).

```ts
import { GateService } from '@buf/minekube_gate.connectrpc_es/minekube/gate/v1/gate_service_connect.js';
```

:::

## Sample project

This sample project is located in the [`docs/developers/api/typescript/node`](https://github.com/minekube/gate/tree/master/.web/docs/developers/api/typescript/node) directory.

To install dependencies:

```bash
pnpm install
```

To run:

```bash
node --experimental-strip-types index.ts
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

This project was created using `pnpm init` in pnpm v9.5.0. [pnpm](https://pnpm.io) is a fast, disk space efficient package manager.
