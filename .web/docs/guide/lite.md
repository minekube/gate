# Gate Lite Mode

## What is Lite mode?

Gate has a `Lite` mode that makes Gate act as an ultra-thin lightweight reverse proxy between
the client and the backend server for host based connection forwarding.

Using different domains or subdomains Lite efficiently routes client connections based on the
host address the player joins with.
This allows you to protect multiple backend servers behind a single port to a Gate Lite proxy instance.

Player connections are offloaded to the destined backend server, including ping requests and player authentication.

**Lite mode supports [proxy behind proxy](#proxy-behind-proxy) setups**, but advanced features like backend server switching or proxy commands are no
longer available in this mode and have no effect when extensions use higher level Gate APIs or non-Lite events.

## Host based Routing

If you point your domain to the IP address Gate listens on, you can use the domain name as the host address.
This allows you to use a single Gate and port for multiple backend servers.

Gate Lite mode `routes` is a list of `host` -> `backend` mappings.
For each hostname, Gate will forward the player connection to the first matching backend server.

[![Graph](/images/lite-mermaid-diagram-LR.svg)](https://gate.minekube.com)

In this configuration, **Gate Lite** will route:
- `Player Bob` -> `Backend A (10.0.0.1)`
-  `Player Alice` -> `Backend B (10.0.0.2)`
```yaml config-lite.yml
config:
  lite:
    enabled: true
    routes:
      - host: abc.example.com
        backend: 10.0.0.3:25568
      - host: '*.example.com'
        backend: 10.0.0.1:25567
      - host: [ example.com, localhost ]
        backend: [ 10.0.0.2:25566 ]
```

## Ping Response Caching

Players send server list ping requests to Gate Lite to display the motd (message of the day).
Gate Lite forwards the actual ping-pong response from the backend server based on the configured route.

If the backend was already pinged some seconds ago Gate Lite directly returns the cached ping response.
This reduces the network traffic since less TCP connections must be made to backend servers to fetch the
status.

### Setting cache duration

To keep and reuse the ping response of a backend for `3 minutes` set:

```yaml
config:
  lite:
    enabled: true
    routes:
      - host: abc.example.com
        backend: [ 10.0.0.3:25565, 10.0.0.4:25565 ]
        pingCacheTTL: 3m # or 180s // [!code ++]
```

_TTL - the Time-to-live before evicting the response data from the in-memory cache_

Note that routes can configure multiple random backends and each backend has its own TTL.

### Disabling the cache

Setting the TTL to `-1` disables response caching for this route only.

```yaml
config:
  lite:
    enabled: true
    routes:
      - host: abc.example.com
        backend: 10.0.0.3:25568
        pingCacheTTL: -1 // [!code ++]
```

## Sample config

The Lite configuration is located in the same Gate `config.yml` file under `lite`.

```yaml config-lite.yml
<!--@include: ../../../config-lite.yml -->
```

## Proxy behind proxy

Gate Lite mode supports proxy behind proxy setups meaning you can use another proxy like
Gate, BungeeCord or Velocity as a backend server.

To preserve the real player IP address you should enable `proxyProtocol: true` or `realIP: true`
(if using TCPShield) for the route as well as on the backend server.

```yaml config-lite.yml
config:
  lite:
    enabled: true
    routes:
      - host: abc.example.com
        backend: 10.0.0.3:25566
        proxyProtocol: true // [!code ++]
```

- [Gate - Enable Proxy Protocol](https://github.com/minekube/gate/blob/7b03987bcdc7e8a6ed96156fa147bdd9dbf6ba4c/config.yml#L85)
- [Velocity - Enable Proxy Protocol](https://docs.papermc.io/velocity/configuration#advanced-section)
- [BungeeCord - Enable Proxy Protocol](https://www.spigotmc.org/wiki/bungeecord-configuration-guide/)
- [Paper - Enable Proxy Protocol](https://docs.papermc.io/paper/reference/global-configuration#proxy-protocol)

## Security considerations

If you use Lite mode and your backend servers do player authentication,
you do not need to worry.

Checkout the [Anti-DDoS](security#ddos-protecting-your-minecraft-server) guide for how
to protect your Minecraft servers from DDoS attacks.
