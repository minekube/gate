---
title: 'Gate Auto Config Reload - Live Configuration Updates'
description: 'Learn which Gate settings the config file watcher applies live, and which changes need a restart.'
---

# Auto Config Reload

_Gate watches your config file for updates._

---

Gate watches your config file and applies the changes that are safe to apply without restarting the proxy
or disconnecting players. Any other change is refused, and the running proxy keeps the last valid config.

::: warning
Only routes in an already-enabled [Lite mode](/guide/lite) configuration are applied live.
A change to anything else - including the whole `bedrock:` block - requires a restart.
:::

## What is applied live

| Change | Applied live? |
| --- | --- |
| `lite.routes` (host matching, backend, route settings) while Lite mode is already enabled | yes |
| Turning Lite mode itself on or off (`lite.enabled`) | no, restart required |
| Any `bedrock` setting | no, restart required |
| Every other setting: `bind`, `onlineMode`, `forwarding`, `servers`, `try`, `forcedHosts`, `status`, `compression`, `quota`, `connect`, `api`, ... | no, restart required |

A route change applies to new connections; players already connected on that route stay connected.

### The `bedrock` block requires a restart

Bedrock support is wired once at startup: the Geyser integration, the Floodgate key and the
link-promotion trust switch are read before any listener starts. The file watcher does not apply any of
the following, and a candidate that changes only one of them is still rejected as a whole:

- `bedrock.enabled`
- `bedrock.geyserListenAddr`
- `bedrock.usernameFormat` and `bedrock.preserveDotPrefix`
- `bedrock.floodgateKeyPath`
- `bedrock.managed.*`
- `bedrock.backendFloodgate.enabled` and `bedrock.backendFloodgate.allowedServers`

That matters in practice: if you toggle `bedrock.backendFloodgate.enabled` and expect it to take effect,
the proxy keeps running the old value. The watcher says so - a refused candidate logs
`config reload rejected` with `reason=unsupported` (at most once per minute per reason) and nothing is
applied. Restart Gate, then confirm the new state; for linked-identity promotion the per-join decision
line described in [Bedrock Support](/guide/bedrock#linked-java-identity) is the evidence.

## How it works

Gate watches your config file for changes and validates a candidate before it applies it.
This is a safe operation: an invalid candidate is rejected, and an unsupported one (see above) is
rejected too. Either way the proxy continues to run with the last valid config and nothing is partially
applied.

## Switching to Lite mode and Connect

Enabling Lite mode, or [Connect](/guide/connect), is a startup change: stop Gate, edit the config,
start it again. Once Lite mode is enabled, its routes can be edited in place without a restart.

## How to enable it

This feature is always enabled by default, given that you have a config file.

## How to disable it

You can disable auto config reload in several ways:

### Command line flag

```bash
gate --no-auto-reload
```

### Environment variable

```bash
GATE_NO_AUTO_RELOAD=true gate
```

### Config file

Add to your `config.yml`:

```yaml
noAutoReload: true
```

This is useful in environments where file watching is not available or causes permission issues (e.g., NixOS with sops-nix).
