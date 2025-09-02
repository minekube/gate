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
After that, just enable connect and add the endpoint to your configuration:

::: code-group

```yaml [config.yml]
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

```json [connect.json]
{ "token": "YOUR-TOKEN" }
```

:::

Then you need to set the `CONNECT_TOKEN` environment variable or create the `connect.json` file next to your config.

## Offline Mode Support

Gate's Connect integration supports offline mode (cracked) players! This allows players without premium Minecraft accounts to join your server through the Connect network.

### Enabling Offline Mode for Connect

To allow offline mode players to join through Connect, add the `allowOfflineModePlayers` option to your Connect configuration:

::: code-group

```yaml [config.yml]
connect:
  enabled: true
  name: my-server-name
  # Allow offline mode (cracked) players to join through Connect
  allowOfflineModePlayers: true // [!code ++]
```

:::

### Important Configuration Notes

When using offline mode with Connect, you have several configuration options for authentication:

::: code-group

```yaml [Gate config.yml]
config:
  # You can keep online mode enabled on Gate
  onlineMode: true
  # Force key authentication can stay enabled or disabled
  forceKeyAuthentication: true # or false
```

```properties [server.properties]
# Must be false when using Gate proxy (Gate handles authentication)
online-mode=false
# Required to be false for offline mode players and chat compatibility
enforce-secure-profile=false
```

:::

::: tip Authentication Flow
When `allowOfflineModePlayers: true` is set, Connect handles the connection injection, allowing offline mode players to join even when Gate has `onlineMode: true`. This provides the best of both worlds - premium players can still authenticate when joining Gate directly while offline players can still join through Connect in offline mode.
:::

## Troubleshooting

### Quick Fixes

Most Connect issues are resolved by ensuring proper backend server configuration:

::: code-group

```properties [server.properties]
# Must be false when using Gate proxy (Gate handles authentication)
online-mode=false
# Required to be false for offline mode players and chat compatibility
enforce-secure-profile=false
```

:::

### Common Issues and Solutions

::: details Authentication/Connection Errors

![Connect authentication error](/images/connect-offline-kicked-profile-key.png)

**Symptoms**:

- "Invalid signature for profile public key"
- Players kicked when joining through Connect
- Direct connection works but Connect doesn't

**Solutions**:

1. **Force endpoint refresh** - Change your endpoint name to clear Connect's cache
2. **Verify configuration** - Check `allowOfflineModePlayers: true` is set
3. **Validate token** - Ensure your Connect token is valid and endpoint is registered

:::

::: details Chat/Communication Issues

![Chat disabled due to missing profile public key](/images/connect-offline-chat-disabled.png)

**Symptom**: Players can join but cannot send chat messages ("Chat disabled due to missing profile public key")

**Solution**: Already covered in backend configuration above - ensure `enforce-secure-profile=false`

:::

::: details Configuration Changes Not Applied

**Symptoms**:

- Offline players still can't join after enabling `allowOfflineModePlayers`
- Changes don't seem to take effect

**Solutions**:

1. **Wait** - Allow 2-3 minutes for Connect network to update
2. **Restart Gate** - Force re-registration with new settings
3. **Change endpoint name** - Force fresh registration if caching persists

:::

### Getting Help

If you encounter issues not covered above:

1. **Check the logs** - Both Gate and server logs often contain helpful error messages
2. **Community support** - Join the [Gate Discord](https://minekube.com/discord) for help
3. **GitHub issues** - Report bugs with logs and reproduction steps on the [Gate repository](https://github.com/minekube/gate/issues)
