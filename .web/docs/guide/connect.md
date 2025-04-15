# Enabling Connect Integration

Gate has a builtin integration with [Connect](https://connect.minekube.com/) to list your proxy on
the [Connect network](https://connect.minekube.com/guide/#the-connect-network).

Great side effect is that it also exposes your locally running Gate proxy to the Internet
and allows players to connect to it from anywhere using the free provided domain
`<my-server-name>.play.minekube.net`.

::: tip The first step is optional
Creating a new endpoint in the dashboard is optional, you can just enter an endpoint name and the token will be created automatically if the name is not already in use (in which case you will get an error). 
Or, if you already have a token, you can skip the first step.
:::

First, go to the [Connect Dashboard](https://app.minekube.com), create a new endpoint and give it a name.<br>
After that, just enable connect and add the endpoint to your `config.yml`:

```yaml Gate config.yml
# Configuration for Connect, a network that organizes all Minecraft servers/proxies
# and makes them universally accessible for all players.
# Among a lot of other features it even allows players to join locally hosted
# Minecraft servers without having an open port or public IP address.
#
# Visit https://developers.minekube.com/connect
connect:
  # Enabling Connect makes Gate register itself to Connect network.
  # This feature is disabled by default, but you are encouraged to
  # enable it and get empowered by the additional network services
  # and by the growing community in this ecosystem.
  enabled: false // [!code --]
  enabled: true // [!code ++]
  # The endpoint name is a globally unique identifier of your server.
  # If Connect is enabled, but no name is specified a random name is
  # generated on every restart (only recommended for testing).
  #
  # It is supported to run multiple Gates on the same endpoint name for load balancing
  # (use the same connect.json token file from first Gate instance).
  #name: your-endpoint-name // [!code --]
  name: my-server-name // [!code ++]
```

Then you neeed to set the `CONNECT_TOKEN` environment variable or create a `connect.json` next to your config with the following format:

```json
{"token":"YOUR-TOKEN"}
```
