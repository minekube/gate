# Security considerations

Gate is a Minecraft reverse proxy that connects players to backend servers.
This means that the backend server might be exposed to the Internet if they listen on public IPs
and players could bypass your Gate instance without authentication.

To prevent this, you should configure your backend servers to only listen on private IPs and/or
use a firewall to only allow connections from your Gate instance. You can also enable modern
forwarding mode _(Velocity mode)_ with using a secret and PaperMC to prevent players from
connecting to your backend servers directly.

::: tip

This does not apply to [Lite mode](lite), where backend servers should do the authentication.

:::
