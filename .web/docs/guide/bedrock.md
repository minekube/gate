---
title: "Gate Bedrock Edition Support - Cross-Platform Gaming"
description: "Enable Minecraft Bedrock Edition support with Gate proxy. Connect Java and Bedrock players on the same network."
---

# Bedrock Edition Support

Enable cross-play between Java and Bedrock players on your Minecraft servers with **zero backend plugins required**.

## 🚀 Quick Start (30 Seconds)

Get Bedrock support running instantly with managed mode:

:::: code-group

```yaml [config.yml]
config:
  bind: 0.0.0.0:25565
  onlineMode: true
  servers:
    server1: localhost:25566
  try:
    - server1

  # Enable Bedrock support - that's it!
  bedrock:
    managed: true
```

::::

```bash
# Start Gate - everything else is automatic!
gate --config config.yml
```

**Connection Info:**

- **Java Players**: `localhost:25565`
- **Bedrock Players**: `localhost:19132` (default, customizable via config overrides)

::: tip Zero Configuration Required!
Gate automatically generates encryption keys, downloads Geyser, creates optimized configs, and manages everything for you. The `managed: true` shorthand enables both Bedrock support and managed mode in one line!
:::



## 🏗️ How It Works

Gate's Bedrock support uses a **proxy-in-front-of-proxy** architecture with built-in Floodgate protocol support:

[![](https://mermaid.ink/img/pako:eNpVks-O0zAQxl_FMheQ0hI3btJEaKVNWlZCLKooEhKEgxtPU2tdu3Kc1War3rggIXHgj7hx4gG48Ty8ADwCbpIuxSfb3_y-GY9nhwvNASe4NGy7Rk-f5wq5df46x3--ffyBUuBGF1doLlkDpnq0NA_P7l_qpZDgoUyrSh82L4Ui_oMcv0GDwRlKW_jTW3QBTQUGLSxTnEmtoMPnRltdaIleGKYqyazQ6gB3qdPWI2s93r9DF8wCcsRN08FpLaQdCIUeS6156dR_aNai0xb98v33zw8oZcUVKI4WYK7vyn-m0VbWpVAVUgAceOfQecwc_evrZ_SEXbP_Xz3PussZF8eKu1KPaGUbCegcrYSUyT2fR0EUnSpprxDK_Mn4VMl6ZRXEBRmdKtNemQCljJ8qs14JaDzhS-zhDZgNE9z95e4Ql2O7hg3kOHFbKcq1zXGu9i6Q1VYvGlXgxJoaPGx0Xa5xsmKycqd6y11Tp4K5idjc3W6ZeqX15oiU5pCox12DwWS6VhYnJA7aYJzs8A1ORsGQhDSMRjQgMY1pFHm4cVEkGvpjOvbJOAhHkR_SvYdvW39_OBmFk3EYxSSOAkKoh8E1XJvLbk7bcd3_Bcao3Os?type=png)](https://mermaid.live/edit#pako:eNpVks-O0zAQxl_FMheQ0hI3btJEaKVNWlZCLKooEhKEgxtPU2tdu3Kc1War3rggIXHgj7hx4gG48Ty8ADwCbpIuxSfb3_y-GY9nhwvNASe4NGy7Rk-f5wq5df46x3--ffyBUuBGF1doLlkDpnq0NA_P7l_qpZDgoUyrSh82L4Ui_oMcv0GDwRlKW_jTW3QBTQUGLSxTnEmtoMPnRltdaIleGKYqyazQ6gB3qdPWI2s93r9DF8wCcsRN08FpLaQdCIUeS6156dR_aNai0xb98v33zw8oZcUVKI4WYK7vyn-m0VbWpVAVUgAceOfQecwc_evrZ_SEXbP_Xz3PussZF8eKu1KPaGUbCegcrYSUyT2fR0EUnSpprxDK_Mn4VMl6ZRXEBRmdKtNemQCljJ8qs14JaDzhS-zhDZgNE9z95e4Ql2O7hg3kOHFbKcq1zXGu9i6Q1VYvGlXgxJoaPGx0Xa5xsmKycqd6y11Tp4K5idjc3W6ZeqX15oiU5pCox12DwWS6VhYnJA7aYJzs8A1ORsGQhDSMRjQgMY1pFHm4cVEkGvpjOvbJOAhHkR_SvYdvW39_OBmFk3EYxSSOAkKoh8E1XJvLbk7bcd3_Bcao3Os)

### The Flow

1. **Bedrock Players** connect to Geyser on UDP port 19132 (default, customizable)
2. **Geyser** translates Bedrock protocol to Java Edition and forwards to Gate
3. **Gate** receives translated connections, handles Floodgate authentication internally, and presents them as regular Java players to backend servers
4. **Backend servers** see all players as normal Java Edition connections - no plugins required!

### Key Benefits

- ✅ **No backend plugins** - Gate handles all Bedrock logic internally
- ✅ **Zero configuration** - Managed mode handles everything automatically
- ✅ **Cross-platform** - Supports all Bedrock platforms (mobile, console, Windows)
- ✅ **Secure** - Uses AES-128 encryption for player authentication



## ⚙️ Configuration Guide

### Basic Configuration

For most users, managed mode provides the perfect balance of simplicity and control:

:::: code-group

```yaml [Minimal Setup]
bedrock:
  managed: true
```

```yaml [With Customization]
bedrock:
  # Custom username format to avoid conflicts
  usernameFormat: '.%s' # .Steve instead of Steve

  # Custom listen address for Geyser connections (localhost for security)
  geyserListenAddr: 'localhost:25567'

  # Custom key path (optional - auto-generated if not specified)
  floodgateKeyPath: '/path/to/key.pem'

  managed:
    enabled: true
    autoUpdate: true # Keep Geyser up-to-date automatically
```

```yaml [Alternative Shorthand]
bedrock:
  managed: true # Implies both enabled: true and managed.enabled: true
  usernameFormat: '.%s'
  geyserListenAddr: 'localhost:25567'
```

::::

### Configuration Options

| Option             | Description                                                 | Default              |
| ------------------ | ----------------------------------------------------------- | -------------------- |
| `usernameFormat`   | Format string for Bedrock usernames (use `%s` for username) | `".%s"`              |
| `geyserListenAddr` | Address where Gate listens for Geyser connections           | `localhost:25567`    |
| `floodgateKeyPath` | Path to Floodgate encryption key                            | `floodgate.pem`      |

::: tip geyserListenAddr Network Configuration

**Default `localhost:25567`** works for most setups where Geyser runs on the same machine.

**Use `0.0.0.0:25567` for:**

- 🐳 Docker Compose with separate containers
- 🌐 Remote Geyser on different server
- ☁️ Kubernetes pod-to-pod communication

Note: All connections are authenticated via Floodgate keys regardless of the binding address.

:::

### Managed Mode Options

| Option       | Description                        | Default   |
| ------------ | ---------------------------------- | --------- |
| `enabled`    | Enable automatic Geyser management | `false`   |
| `autoUpdate` | Automatically update Geyser JAR    | `true`    |
| `javaPath`   | Path to Java executable            | `java`    |
| `dataDir`    | Directory for Geyser files         | `.geyser` |
| `extraArgs`  | Additional JVM arguments           | `[]`      |

### Configuration Modes

Gate supports two approaches for Bedrock integration:

#### Managed Mode (Recommended)

Gate automatically handles Geyser for you:

**Shorthand syntax:**

```yaml
bedrock:
  managed: true # Simplest - enables everything automatically
```

**Explicit syntax (equivalent):**

```yaml
bedrock:
  enabled: true
  managed:
    enabled: true
```

#### Manual Mode (Advanced)

You manage your own Geyser installation:

```yaml
bedrock:
  enabled: true
  floodgateKeyPath: '/path/to/key.pem'
  # managed: false (default when omitted)
```

| Mode        | Complexity | Control | Best For                     |
| ----------- | ---------- | ------- | ---------------------------- |
| **Managed** | Simple     | Medium  | Most users, quick setup      |
| **Manual**  | Medium     | Full    | Advanced users, custom needs |

## 🔧 Advanced Configuration

### Custom Geyser Settings

Override any Geyser configuration option using `configOverrides`. For a complete list of available Geyser settings, see the [GeyserMC Configuration Guide](https://geysermc.org/wiki/geyser/understanding-the-config/).

:::: code-group

```yaml [Performance Tuning]
bedrock:
  managed:
    enabled: true
    configOverrides:
      # Optimize for performance
      bedrock:
        port: 19132 # Custom Bedrock port (defaults to 19132)
        compression-level: 8
        mtu: 1200
      use-direct-connection: true
      disable-compression: false
      max-players: 500
```

```yaml [Custom Branding]
bedrock:
  managed:
    enabled: true
    configOverrides:
      # Customize server branding
      bedrock:
        motd1: 'My Amazing Server'
        motd2: 'Cross-Play Enabled!'
        server-name: 'MyServer Bedrock'
      xbox-achievements-enabled: true
```

```yaml [Debug Mode]
bedrock:
  managed:
    enabled: true
    configOverrides:
      # Enable debugging
      debug-mode: true
      log-player-ip-addresses: false
      notify-on-new-bedrock-update: false
```

```yaml [Custom Port with Shorthand]
bedrock:
  managed: true
  configOverrides:
    # Use a different Bedrock port
    bedrock:
      port: 25565 # Use same port as Java (if on different IPs)
```

::::

### Username Formatting

Prevent conflicts between Java and Bedrock usernames:

:::: code-group

```yaml [Prefix with Dot]
bedrock:
  managed: true
  usernameFormat: '.%s' # Steve becomes .Steve
```

```yaml [Suffix with Platform]
bedrock:
  managed: true
  usernameFormat: '%s_BE' # Steve becomes Steve_BE
```

```yaml [Custom Format]
bedrock:
  managed: true
  usernameFormat: 'Mobile_%s' # Steve becomes Mobile_Steve
```

::::

### Manual Setup (Advanced)

For users who want to manage their own Geyser installation:

:::: code-group

```yaml [Gate Configuration]
bedrock:
  enabled: true
  # Geyser will connect to this address (localhost for same-machine, 0.0.0.0 for Docker/remote)
  geyserListenAddr: 'localhost:25567'
  # Username format for Bedrock players
  usernameFormat: '.%s'
  # Path to shared Floodgate key
  floodgateKeyPath: '/path/to/key.pem'
  # managed: false (default when omitted)
```

```yaml [Geyser config.yml]
# Geyser Standalone configuration
bedrock:
  # UDP port for Bedrock players
  port: 19132
  address: 0.0.0.0

remote:
  # Connect to Gate's Bedrock listener
  address: localhost
  port: 25567
  auth-type: floodgate
  use-proxy-protocol: true

# Point to shared Floodgate key
floodgate-key-file: /path/to/key.pem

# Enable passthrough for better integration
passthrough-motd: true
passthrough-player-counts: true

# Performance settings
max-players: 100
debug-mode: false
```

::::

**Setup Steps:**

1. **Generate Floodgate key** (if you don't have one):

```bash
   # Generate 16-byte AES-128 key
openssl rand -out key.pem 16
   chmod 600 key.pem
```

2. **Download Geyser Standalone**:

```bash
   # Download latest Geyser Standalone
   wget https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone -O geyser-standalone.jar
```

3. **Configure both Gate and Geyser** with the examples above

4. **Start in correct order**:

```bash
   # 1. Start Gate first
   gate --config config.yml

   # 2. Start your backend servers
   # (with the shared key.pem if using Floodgate plugin)

   # 3. Start Geyser Standalone
   java -Xmx1G -jar geyser-standalone.jar
```

#### When to Use Manual Mode

Consider manual setup if you:

- Need custom Geyser configurations not supported by `configOverrides`
- Want to run Geyser on a different server/container
- Require specific Geyser versions or custom builds
- Need to integrate with existing orchestration systems
- Want full control over Geyser's lifecycle and resources

#### Manual Mode Considerations

- **Startup order matters**: Gate must start before Geyser connects
- **Key management**: You're responsible for generating and securing Floodgate keys
- **Updates**: You must manually update Geyser JAR files
- **Configuration sync**: Gate and Geyser configs must stay coordinated
- **Monitoring**: You need to monitor both Gate and Geyser processes

::: warning Manual Setup Complexity
Manual setup requires careful coordination of configurations, startup order, and key management. **Managed mode is recommended** for most users as it handles all of this automatically.
:::

### Docker Compose Setup

For containerized deployments:

:::: code-group

```yaml [docker-compose.yml]
<!--@include: ../../../.examples/bedrock/docker-compose.yml -->
```

```yaml [gate.yml]
<!--@include: ../../../.examples/bedrock/gate.yml -->
```

::::

```bash
# Clone and start the stack
git clone https://github.com/minekube/gate.git
cd gate/.examples/bedrock
docker compose up -d
```

::: tip Network Address Considerations

**Default: `localhost:25567`** (recommended for same-machine setups)

- ✅ **Local installations** - Gate and Geyser on same server
- ✅ **Managed mode** - Gate automatically runs Geyser locally
- ✅ **Simplicity** - No network configuration needed

**Use `0.0.0.0:25567` for:**

- 🐳 **Docker Compose** - Gate and Geyser in separate containers
- 🌐 **Remote Geyser** - Geyser runs on a different machine
- ☁️ **Kubernetes** - Pods communicate across network

The Docker example above uses `gate:25567` (service name) which is correct for container networks.

All connections require valid Floodgate keys for authentication.

:::

---

## 🔬 Internals & System Architecture

_For developers who want to understand how Gate's Bedrock support works under the hood._

### Managed Mode Architecture

Gate's managed mode represents a complete lifecycle management system for Geyser Standalone. When enabled, Gate becomes responsible for every aspect of Geyser's operation, from initial setup to graceful shutdown. This approach eliminates the complexity of manual Geyser configuration while providing developers with full control through configuration overrides.

The managed system operates through five core phases: **automatic key generation**, **intelligent JAR management**, **dynamic configuration generation**, **process orchestration**, and **ready state detection**. Each phase is designed to handle edge cases and failure scenarios gracefully, ensuring that Bedrock support remains robust even in challenging deployment environments.

#### Automatic Key Generation

Gate implements cryptographically secure key generation that follows Floodgate's exact specifications. When no encryption key exists, the system automatically creates a 16-byte AES-128 key using the operating system's secure random number generator. This key serves as the shared secret between Geyser and Gate for authenticating Bedrock player data.

The key generation process includes several security measures: secure file permissions are set to prevent unauthorized access, parent directories are created automatically to handle complex deployment structures, and the key format exactly matches what Floodgate expects. This eliminates the common configuration errors that occur when keys are generated manually or copied incorrectly between systems.

#### Smart JAR Management

Gate's JAR management system implements intelligent HTTP caching to minimize bandwidth usage and startup time. Rather than downloading Geyser on every startup, the system uses HTTP conditional requests with ETag and Last-Modified headers to determine if updates are available. This approach respects Geyser's distribution server while ensuring users always run the latest compatible version.

The download system includes comprehensive error handling, timeout management, and integrity verification. When updates are available, downloads happen in the background with progress logging, and the system gracefully handles network failures or corrupted downloads by falling back to cached versions when possible.

#### Configuration Generation & Deep Merging

Gate generates optimized Geyser configurations tailored specifically for proxy integration. The base configuration includes performance tunings discovered through extensive testing, proper proxy protocol settings, and security configurations that work seamlessly with Gate's authentication system.

User customizations are applied through a deep merging algorithm that preserves the structure of nested configuration options. This means users can override specific settings like `bedrock.compression-level` without affecting other `bedrock` section options, providing fine-grained control while maintaining sensible defaults for unconfigured options.

#### Process Orchestration & Ready Detection

Gate manages Geyser as a child process, handling all aspects of lifecycle management including startup argument construction, environment setup, and graceful shutdown coordination. The system monitors Geyser's output streams to detect when the service becomes ready to accept connections, ensuring that Gate doesn't route traffic before Geyser is prepared to handle it.

The process management includes automatic restart capabilities on configuration changes, resource cleanup on shutdown, and comprehensive logging integration that merges Geyser's output with Gate's logging system for unified troubleshooting.

### Floodgate Protocol Implementation

Gate includes a complete, native implementation of the Floodgate protocol, eliminating the need for backend server plugins. This implementation handles the complex cryptographic operations required to securely authenticate Bedrock players and extract their platform-specific information.

#### Bedrock Player Data Processing

The Floodgate protocol encodes comprehensive player information in an encrypted, structured format. Gate's implementation can decode this data to extract player usernames, Xbox User IDs (XUIDs), device platform information, language preferences, input methods, and other metadata that helps servers provide platform-appropriate experiences.

The data extraction process includes robust validation to prevent malformed or malicious data from affecting server operation. Each field is validated according to Floodgate's specification, with appropriate error handling for edge cases like missing usernames or invalid XUIDs.

#### Programmatic Access to Bedrock Data

For developers building Gate plugins or extensions, Gate provides direct access to Bedrock player information through the context system. This allows you to create platform-specific features and optimizations in your Go code.

```go
import (
    "go.minekube.com/common/minecraft/component"
    "go.minekube.com/gate/pkg/edition/bedrock/geyser"
    "go.minekube.com/gate/pkg/edition/java/proxy"
)

func handlePlayerJoin(event *proxy.PostLoginEvent) {
    player := event.Player()

    // Check if player is from Bedrock Edition
    if bedrockData := geyser.FromContext(player.Context()); bedrockData != nil {
        // This is a Bedrock player - access device info
        if bedrockData.DeviceOS == geyser.DeviceOSAndroid {
            player.SendMessage(&component.Text{Content: "Welcome mobile player!"})
        }

        // Access other Bedrock data:
        // bedrockData.Username, bedrockData.Xuid, bedrockData.DeviceOS,
        // bedrockData.InputMode, bedrockData.Language, etc.
    }
}
```

This programmatic access enables sophisticated cross-platform features like platform-specific optimizations, and targeted messaging based on the player's device capabilities.

#### Deterministic UUID Generation

One of the most critical aspects of cross-platform play is ensuring Bedrock players receive consistent Java Edition UUIDs across sessions. Gate implements a deterministic UUID generation algorithm that creates RFC 4122-compliant UUIDs from Bedrock XUIDs using cryptographic hashing.

This approach ensures that the same Bedrock player always receives the same Java UUID, enabling proper player data persistence, permissions systems, and plugin compatibility. The algorithm uses SHA1 hashing with a Floodgate-specific namespace to prevent UUID collisions while maintaining deterministic behavior.

### Comprehensive Device Detection

Gate's device detection system provides servers with detailed information about player platforms, enabling platform-specific features and optimizations. The system recognizes all major Bedrock platforms including mobile devices, gaming consoles, desktop clients, and emerging platforms.

#### Platform Classification Intelligence

The device detection goes beyond simple platform identification to provide intelligent categorization. The system understands that Amazon Fire devices run Android-based Fire OS, that Samsung Gear VR operates on Android, and that different input methods (touch, controller, keyboard) affect gameplay mechanics.

This intelligence enables servers to make informed decisions about features like UI scaling, control schemes, and performance optimizations. For example, a server might enable simplified controls for mobile players while providing full keyboard shortcuts for desktop users.

#### Cross-Platform Compatibility Handling

The detection system includes special handling for platform-specific quirks and limitations. It understands console-specific behaviors, mobile device performance constraints, and the unique characteristics of different Bedrock client implementations. This knowledge helps Gate provide appropriate translations and optimizations for each platform.

The system also handles edge cases like players switching between devices, platform spoofing attempts, and the introduction of new Bedrock platforms through a flexible, extensible architecture that can adapt to Mojang's evolving Bedrock ecosystem.

---

## 🔍 Troubleshooting

### Common Issues

:::: details Bedrock Players Can't Connect

**Symptoms:**

- "Unable to connect to world" on Bedrock clients
- Geyser shows connection timeouts

**Solutions:**

1. **Check UDP port** - Ensure port 19132 is open for UDP traffic:

   ```bash
   # Test UDP port accessibility
   nc -u -l 19132  # On server
   nc -u server-ip 19132  # From client
   ```

2. **Verify managed mode status** - Check Gate logs for Geyser startup:

   ```log
   INFO bedrock.managed geyser standalone process started pid=1234
   INFO [GEYSER] Done (5.2s)! Run /geyser help for help!
   ```

3. **Check firewall** - Allow UDP 19132 and TCP 25567:
   ```bash
   sudo ufw allow 19132/udp  # Bedrock clients
   sudo ufw allow 25567/tcp  # Geyser to Gate
   ```

::::

:::: details Authentication Errors

**Symptoms:**

- "Failed to verify username" in logs
- Players kicked during login with authentication errors

**Solutions:**

1. **Verify key generation** - Check if Floodgate key was created:

   ```bash
   ls -la floodgate.pem
   # Should show: -rw------- (0600 permissions)
   # Should be exactly 16 bytes
   ```

2. **Check key permissions** - Ensure Gate can read the key:

   ```bash
   chmod 600 floodgate.pem
   chown gate:gate floodgate.pem
   ```

3. **Validate key format** - Regenerate if corrupted:
   ```bash
   # Delete old key and restart Gate (auto-generates new one)
   rm floodgate.pem
   gate --config config.yml
   ```

::::

:::: details Performance Issues

**Symptoms:**

- High latency for Bedrock players
- Server lag when Bedrock players join

**Solutions:**

1. **Tune Geyser settings** - Add performance overrides:

   ```yaml
   bedrock:
     managed:
       enabled: true
       configOverrides:
         bedrock:
           compression-level: 8 # Higher compression
           mtu: 1200 # Optimize packet size
         use-direct-connection: true
         disable-compression: false
   ```

2. **Increase memory** - Add JVM args for Geyser:

   ```yaml
   bedrock:
     managed:
       enabled: true
       extraArgs: ['-Xmx2G', '-XX:+UseG1GC']
   ```

3. **Network optimization** - Reduce network overhead:
   ```yaml
   bedrock:
     managed:
       enabled: true
       configOverrides:
         scoreboard-packet-threshold: 20
         enable-proxy-connections: false
   ```

::::

### Manual Setup Issues

:::: details Geyser Can't Connect to Gate

**Symptoms:**

- Geyser shows "Connection refused" or timeout errors
- Geyser fails to connect to Gate's listener

**Solutions:**

1. **Check startup order** - Gate must be running before Geyser:

   ```bash
   # Verify Gate is listening on the configured port
   netstat -tlnp | grep 25567
   ```

2. **Verify configuration alignment**:

   ```yaml
   # Gate config - geyserListenAddr
   bedrock:
     geyserListenAddr: 'localhost:25567'

   # Geyser config - remote.port must match
   remote:
     address: localhost
     port: 25567
   ```

3. **Check firewall/networking**:
   ```bash
   # Test TCP connectivity from Geyser to Gate
   telnet gate-host 25567
   ```

::::

:::: details Floodgate Key Errors

**Symptoms:**

- "Failed to decrypt bedrock data" in Gate logs
- Authentication failures for Bedrock players

**Solutions:**

1. **Verify key file paths match**:

   ```bash
   # Same key must exist at both locations
   ls -la /path/to/key.pem  # Gate's floodgateKeyPath
   ls -la /geyser/key.pem   # Geyser's floodgate-key-file
   ```

2. **Check key file permissions**:

   ```bash
   chmod 600 key.pem
   chown gate:gate key.pem  # For Gate process
   chown geyser:geyser key.pem  # For Geyser process
   ```

3. **Regenerate if corrupted**:
   ```bash
   # Generate new 16-byte key
   openssl rand -out key.pem 16
   # Copy to both Gate and Geyser locations
   cp key.pem /path/to/gate/floodgate.pem
   cp key.pem /path/to/geyser/key.pem
   ```

::::

:::: details Configuration Sync Issues

**Symptoms:**

- Players can connect but experience issues
- Inconsistent behavior between Java and Bedrock players

**Solutions:**

1. **Verify proxy protocol settings**:

   ```yaml
   # Geyser MUST use proxy protocol with Gate
   remote:
     use-proxy-protocol: true
   ```

2. **Check authentication mode alignment**:

   ```yaml
   # Both must use floodgate
   remote:
     auth-type: floodgate
   ```

3. **Ensure passthrough settings are correct**:
   ```yaml
   # For best integration with Gate
   passthrough-motd: true
   passthrough-player-counts: true
   ```

::::

### Debug Mode

Enable detailed logging for troubleshooting:

:::: code-group

```yaml [config.yml]
bedrock:
  managed:
    enabled: true
    configOverrides:
      debug-mode: true
      log-player-ip-addresses: true
```

::::

### Getting Help

1. **Check logs** - Gate, Geyser, and backend server logs all contain useful information
2. **Verify versions** - Ensure Gate, Geyser, and server versions are compatible
3. **Community support** - Join the [Gate Discord](https://minekube.com/discord) for help
4. **GitHub issues** - Report bugs with logs and reproduction steps at [gate/issues](https://github.com/minekube/gate/issues)

## 📋 Supported Features

### ✅ Fully Supported

- **Cross-platform play** - All Bedrock devices can join Java servers
- **Authentication** - Secure Xbox Live authentication via Floodgate
- **Chat & commands** - Full compatibility between editions
- **World interaction** - Building, mining, crafting work normally
- **Device detection** - Server can identify player platforms
- **Inventory sync** - Items transfer correctly between editions

### ⚠️ Partial Support

- **Custom items** - Java-specific items may render differently
- **Resource packs** - Bedrock packs need special conversion
- **Some plugins** - Java-specific plugins may not work with Bedrock players

### ❌ Not Supported

- **Bedrock-exclusive features** - Education Edition content, some UI elements
- **Java mods** - Forge/Fabric mods don't work with Bedrock clients
- **Complex redstone** - Some advanced redstone may behave differently

---

_For more information about Geyser and Floodgate, visit the [GeyserMC Wiki](https://wiki.geysermc.org/)._
