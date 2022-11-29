# Documentation Website

This website is built using [Vitepress](https://vitepress.vuejs.org/),
a modern static website generator for documentation.

## Setup

> You must have a recent version of Node.js (14+) installed.
> You may use [Volta](https://github.com/volta-cli/volta), a Node version manager,
> to install the latest version of Node and `yarn`.

```sh console
$ curl https://get.volta.sh | bash
$ volta install node yarn
```

### Installation

Finally, you will need to install the Node.js dependencies for this project
using yarn or another package manager:

```sh console
$ yarn install
```

### Local Development

```sh console
$ yarn run dev/connect
```

This command starts a local development server and opens up a browser window.
Most changes are reflected live without having to restart the server.

### Build

```sh console
$ yarn run build/connect
```

This command generates static content into the `dist` directory and can be served
using any static contents hosting service.

### Deployment

Our docs are deployed using [Cloudflare Pages](https://pages.cloudflare.com).
Every commit pushed to `main` branch will automatically deploy to
[connect.minekube.com](https://connect.minekube.com),
and any pull requests opened will have a corresponding staging URL available in
the pull request comments.
