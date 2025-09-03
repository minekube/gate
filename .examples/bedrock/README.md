# Gate + Geyser Bedrock Support Example

This example demonstrates how to set up Gate with Geyser for Bedrock Edition support, allowing both Java and Bedrock players to join the same server.

## Quick Start

1. **Generate Floodgate Key** (first time only):

   ```bash
   # Generate a new Floodgate key
   docker run --rm -v $(pwd)/geyser:/data itzg/minecraft-server \
     sh -c "mkdir -p /data && openssl genpkey -algorithm RSA -out /data/key.pem -pkcs8"
   ```

2. **Start the services**:

   ```bash
   docker compose up -d
   ```

3. **Check logs**:

   ```bash
   docker compose logs -f
   ```

4. **Connect**:
   - **Java players**: Connect to `localhost:25565`
   - **Bedrock players**: Connect to `localhost:19132`

## Architecture

```
Bedrock Players (19132/udp) → Geyser → Gate (25567) → Backend Servers
Java Players (25565/tcp) → Gate → Backend Servers
```

## Services

- **Gate**: Main proxy server handling both Java and translated Bedrock connections
- **Geyser**: Protocol translator converting Bedrock to Java Edition protocol
- **Server1**: Backend Minecraft server (no plugins required)
- **Volumes**: Persistent world data

## Configuration Files

- `gate.yml` - Gate proxy configuration with Bedrock support enabled
- `geyser/config.yml` - Geyser standalone configuration

- `server.properties` - Backend server properties
- `docker-compose.yml` - Docker services orchestration

## Security Notes

- The Gate Bedrock listener (port 25567) should only accept connections from Geyser
- In production, use firewall rules to restrict access to this port
- The `key.pem` file enables secure authentication between Geyser and Gate (no backend plugins required)

## Troubleshooting

### Bedrock players can't connect

- Check that UDP port 19132 is accessible
- Verify Geyser logs for connection errors
- Ensure the Floodgate key is properly shared

### Authentication errors

- Verify the `key.pem` is accessible by both Geyser and Gate
- Check file permissions on the key file
- Ensure backend servers have `online-mode=false` (since Gate handles authentication)

### Performance issues

- Adjust Geyser's `compression-level` and `mtu` settings
- Monitor resource usage of the Geyser container
- Consider using `use-direct-connection: true` in Geyser config

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
