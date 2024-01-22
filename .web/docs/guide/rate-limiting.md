# Rate limiting

_You can find the rate limiters under the `quota` section of the config._

Rate limiting is an important mechanism for controlling
resource utilization and managing quality of service.

There are two rate limiters:
- Connection limiter
  - triggered upfront on any new connection
- Login limiter
  - triggered just before authenticating player with Mojang
    to prevent flooding the Mojang API

Each rate limiter is IP block based but cutting off
the last numbers (/24 block) as in 255.255.255`.xxx`.

Too many connections from the same IP-block (as configured)
will be simply disconnected, and the default settings should
never affect legitimate players and only rate limit aggressive
behaviours.


::: tip

The limiter only prevents attacks on a per IP block bases
and cannot mitigate against distributed denial of services (DDoS), since this type
of attack should be handled on a higher networking layer than Gate operates in.

See [DDoS Protecting your Minecraft server](/guide/security/ddos) for details.

:::
