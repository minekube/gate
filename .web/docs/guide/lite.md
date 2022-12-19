# Gate Lite mode

## What is Lite mode?

Gate has a `Lite` mode that makes Gate act as an ultra-thin lightweight reverse proxy between
the client and the backend server.

Using different domains or subdomains Lite efficiently routes client connections based on the
host address the player joins with.
This allows you to protect multiple backend servers behind a single port to a Gate Lite proxy instance.

Player connections are offloaded to the destined backend server, including ping requests and player authentication.

**Lite mode supports proxy behind proxy setups**, but advanced features like backend server switching or proxy commands are no
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


## Enabling Lite mode

You can switch to Lite mode by enabling it in the `config.yml`.

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

## Security considerations

If you use Lite mode and your backend servers do player authentication,
you do not need to worry.

Checkout the [Anti-DDoS](security#ddos-protecting-your-minecraft-server) guide for how
to protect your Minecraft servers from DDoS attacks.
