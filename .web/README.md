# Documentation Website

This website is built using [Vitepress](https://vitepress.vuejs.org/),
a modern static website generator for documentation.

## Setup

> You must have Node.js 22+ installed.
> You may use [Volta](https://github.com/volta-cli/volta), a Node version manager,
> to install Node.js and `pnpm`.

```sh console
$ curl https://get.volta.sh | bash
$ volta install node@22 pnpm
```

### Installation

Finally, you will need to install the Node.js dependencies for this project
using pnpm:

```sh console
$ pnpm install
```

### Local Development

```sh console
$ pnpm run dev
```

This command starts a local development server and opens up a browser window.
Most changes are reflected live without having to restart the server.

### Build

```sh console
$ pnpm run build
```

This command generates static content into the `dist` directory and can be served
using any static contents hosting service.

### Deployment

Our docs are deployed using [Cloudflare Pages](https://pages.cloudflare.com).
Every commit pushed to `main` branch will automatically deploy to
[connect.minekube.com](https://connect.minekube.com),
and any pull requests opened will have a corresponding staging URL available in
the pull request comments.

Worker deployments use the pinned Wrangler toolchain. Set
`GITHUB_CACHE_KV_NAMESPACE_ID` to the existing `GITHUB_CACHE` Workers KV
namespace ID in the deployment environment. Before the first uncached API
request, provision the existing GitHub App private key as a secret on each
Worker; the key must remain outside the repository:

```sh console
$ pnpm exec wrangler secret put GITHUB_APP_PRIVATE_KEY --config wrangler.canary.jsonc
$ pnpm exec wrangler secret put GITHUB_APP_PRIVATE_KEY --config wrangler.jsonc
```

Then run `pnpm run build:worker` followed by
`pnpm run deploy:worker:canary`.
The production command is `pnpm run deploy:worker`; Cloudflare Pages remains
available for rollback during canary verification.
