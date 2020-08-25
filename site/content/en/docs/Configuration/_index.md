---
title: "Configure Gate"
linkTitle: "Configuration"
weight: 7
description: >
  This page explains how you can configure Gate
  using a config file or environment variables.
---

## TODO


## Rate limiter

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


{{< alert title="Note" color="info">}}
The limiter only prevents attacks on a per IP block bases
and cannot mitigate against distributed denial of services (DDoS), since this type
of attack should be handled on a higher networking layer than Gate operates in.
{{< /alert >}}