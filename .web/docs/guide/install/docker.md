# Running Gate in Docker

_Gate ships in packaged Docker images that you can use to run Gate containers or base your own images on. You can also
run it in [Kubernetes](kubernetes)._

## Version Tags

You can use specific version tags instead of the latest. Every commit to the `main` branch is built and pushed to
the `latest` tag as well as the commit's short SHA
like [`6d3671c`](https://github.com/minekube/gate/pkgs/container/gate/50952923?tag=6d3671c).

## `docker run`

```sh console
docker run -it --rm ghcr.io/minekube/gate:latest
```

This command will pull and run the latest Gate image.

- `-it` - Run interactively and allocate a pseudo-TTY.
    - Alternatively using `-d` would run in detached mode.
- `--rm` - Removes the container after it exits.

## `docker-compose.yaml`

Copy the following snippet into a `docker-compose.yaml` file and run `docker-compose up`.

```yaml docker-compose.yaml
version: "3.9"

services:
  gate:
    image: ghcr.io/minekube/gate:latest
    container_name: gate
    restart: unless-stopped
    network_mode: host
```

Running `docker-compose down` will stop and remove the containers.

We provide an example that configures Gate with two Minecraft servers.

```sh console
git clone https://github.com/minekube/gate.git
cd gate/.examples/docker-compose
docker-compose up
```

The files of the two servers are located in the `serverdata*` directories.
You can join at `localhost:25565` and use `/server` to switch between the servers.


## Troubleshooting

If you see the following error:

```sh console
Unable to find image 'ghcr.io/minekube/gate:latest' locally
docker: Error response from daemon: Head "https://ghcr.io/v2/minekube/gate/manifests/latest": denied: denied.
See 'docker run --help'.
```

do

```sh console
docker logout ghcr.io
```

It may be because you are logged in to the GitHub Container Registry with an outdated personal access token (PAT).
Simply logout or login with a new token. It's worse to provide a bad token than not to provide a token at all. GitHub
sets tokens to expire after 30 days by default.
