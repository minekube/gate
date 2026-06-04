# Gate + geyserlite Bedrock Support Example

This example demonstrates Gate's managed Bedrock mode. Gate starts geyserlite automatically so Java and Bedrock players can join the same Paper backend.

## Quick Start

1. **Start the services**:

   ```bash
   docker compose up -d
   ```

2. **Check logs**:

   ```bash
   docker compose logs -f
   ```

3. **Connect**:
   - **Java players**: Connect to `localhost:25565`
   - **Bedrock players**: Connect to `localhost:19132`

## Architecture

```
Bedrock Players (19132/udp) → Gate managed geyserlite → Paper
Java Players (25565/tcp) → Gate → Backend Servers
```

## Services

- **Gate**: Main proxy server handling both Java and translated Bedrock connections
- **geyserlite**: Native Geyser engine downloaded and started by Gate managed mode
- **Server1**: Paper `26.1.2` backend running on Java 25
- **Volumes**: Persistent Gate cache/data and Paper world data

## Configuration Files

- `gate.yml` - Gate proxy configuration with `bedrock: true`
- `server.properties` - Backend server properties
- `spigot.yml` - Enables Bungee/legacy forwarding for Gate
- `docker-compose.yml` - Docker services orchestration

## Security Notes

- The backend runs with `online-mode=false` because Gate authenticates players.
- Restrict backend access so players cannot bypass Gate.
- `spigot.yml` enables Bungee/legacy forwarding, which is required for Gate to pass player identity data.

## Troubleshooting

### Bedrock players can't connect

- Check that UDP port 19132 is accessible
- Verify Gate logs for geyserlite startup errors
- Ensure the Gate container can write to its cache/data volumes

### Authentication errors

- Ensure backend servers have `online-mode=false`
- Ensure `settings.bungeecord: true` is present in `spigot.yml`
- Keep backend access restricted to Gate

### Performance issues

- Monitor resource usage of the Gate container
- Configure advanced managed options in `gate.yml` if you need custom geyserlite settings

## Customization

### Username Format

Change the Bedrock username prefix in `gate.yml`:

```yaml
  bedrock:
    usernameFormat: 'BE_%s' # Prefix with "BE_"
```

### Resource Packs

Add Bedrock-compatible resource packs to the Geyser configuration.

### Plugins

Install additional plugins on the backend server. Most Java plugins work with Bedrock players since Gate handles the protocol translation and presents them as regular Java players.

## Production Deployment

For production use:

1. Use proper secrets management for the Floodgate key
2. Configure firewall rules to protect the Bedrock listener port
3. Set up monitoring and logging
4. Use persistent volumes for world data
5. Configure backup strategies
6. Consider using Gate's Connect integration for DDoS protection

## Support

- [Gate Documentation](https://gate.minekube.com/)
- [Geyser Wiki](https://wiki.geysermc.org/)
- [Gate Discord](https://minekube.com/discord)
