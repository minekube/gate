---
title: "Gate Minecraft Proxy Rate Limiting & DDoS Protection"
description: "Configure rate limiting in Gate Minecraft proxy to prevent abuse and attacks. IP-based connection and login limiters for server protection and quality control."
---

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

Each rate limiter uses an IP-prefix bucket: IPv4 addresses share a /24 bucket
(`255.255.255.xxx`), while IPv6 addresses share a /64 bucket (the first four
hextets). IPv4-mapped IPv6 addresses are treated as IPv4 addresses.

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
