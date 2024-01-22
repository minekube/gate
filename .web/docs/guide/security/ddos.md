# Security considerations

Gate is a Minecraft reverse proxy that connects players to backend servers.
This means that the backend server might be exposed to the Internet if they listen on public IPs
and players could bypass your Gate instance without authentication.

To prevent this, you should configure your backend servers to only listen on private IPs and/or
use a firewall to only allow connections from your Gate instance. You can also enable modern
forwarding mode _(Velocity mode)_ with using a secret and PaperMC to prevent players from
connecting to your backend servers directly.

::: tip

In [Lite mode](/guide/lite), the backend servers do the authentication.

:::

## DDoS Protecting your Minecraft server

If you are running a high-profile public Minecraft server, and you are not using [Connect](https://connect.minekube.com),
having a good DDoS protection is essential to prevent your server from being taken offline.

If you are under attack, your server will be lagging, become unresponsive and timeout players.
This is not good for your player experience nor your brand.

There are many ways to protect your server from DDoS attacks.
Here are common methods proven to work very well in production:

### Minekube Connect <VPBadge>free, fast setup & low latency</VPBadge>

Minekube Connect is our free proxy service that provides DDoS protection
for your Minecraft endpoints. It is very easy to set up as you only need to
[enable Connect mode](/guide/connect) in Gate's configuration, or install a plugin to your Java server/proxy,
and update your DNS records if you have a custom domain.

-> [Learn more about Connect Anti-DDoS](https://connect.minekube.com)

### OVHcloud Anti-DDoS <VPBadge>cheap & reliable</VPBadge>

_OVH is a well known service provider that offers a very good
[DDoS protection](https://www.ovhcloud.com/en/security/anti-ddos/) service for your servers._

You don't need to host all your Minecraft servers on OVH, but you can set up
[Gate Lite](/guide/lite) on a tiny [VPS instance](https://www.ovhcloud.com/en/vps/) and
forward all your traffic to your backend servers.

::: details OVH Anti-DDoS setup

1. [Create a VPS instance](https://www.ovhcloud.com/en/vps/) _(any provider with good Anti-DDoS)_
2. [Install Gate](/guide/install/) on your VPS with [Lite mode](/guide/lite) enabled to point to your actual servers (not required)
3. [Activate Anti-DDoS](https://www.ovhcloud.com/en/security/anti-ddos/) in the OVH dashboard for your VPS
4. Configure your DNS to point your domain to your VPS IP address

:::

### Cloudflare Spectrum <VPBadge type='warning'>very costly</VPBadge>

Cloudlare is a well known service provider with global scale DDoS protection
for TCP services using [Cloudflare Spectrum](https://www.cloudflare.com/products/cloudflare-spectrum/minecraft/)


### TCPShield <VPBadge type='danger'>uses OVH</VPBadge>

TCPShield is a Minecraft proxy service that uses OVH's DDoS protection.
It is free for small servers.
