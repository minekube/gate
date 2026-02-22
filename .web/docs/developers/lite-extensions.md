---
title: "Lite Extensions"
description: "Write Gate Lite extensions using Lite plugin hooks, forward events, and active forward queries."
---

# Lite Extensions

Gate Lite now exposes a small, generic extension surface for observability-focused integrations.

Use `lite.Plugin` when your code needs to run in Lite mode and only needs:

- Lite lifecycle initialisation
- Lite forward start/end events
- Read-only active forward lookups

## Lite Plugin Lifecycle

`lite.Plugin` is separate from `proxy.Plugin`.

- `proxy.Plugin` targets the full Java proxy APIs (players, commands, server switching, etc.)
- `lite.Plugin` targets Lite mode runtime observability only

Lite plugins are initialised only when `config.lite.enabled: true`.

```go
lite.Plugins = append(lite.Plugins, lite.Plugin{
    Name: "MyLitePlugin",
    Init: func(ctx context.Context, rt *lite.Runtime) error {
        return nil
    },
})
```

## Available Lite Events

Subscribe using the Lite runtime event manager:

```go
event.Subscribe(rt.Event(), 0, func(e *lite.ForwardStartedEvent) {})
event.Subscribe(rt.Event(), 0, func(e *lite.ForwardEndedEvent) {})
```

### `lite.ForwardStartedEvent`

Emitted when a Lite TCP forward starts after backend selection and connection setup succeeds.

Important fields:

- `ConnectionID`
- `ClientIP` (effective IP, best effort)
- `ClientAddr` (raw remote address)
- `BackendAddr` (selected backend)
- `Host` (requested virtual host)
- `RouteID` (derived route identifier)
- `StartedAt`

### `lite.ForwardEndedEvent`

Emitted on terminal forward paths, including backend connect failure before a stream starts.

Additional fields:

- `EndedAt`
- `Reason`

`Reason` values include:

- `ClientClosed`
- `BackendClosed`
- `BackendConnectFailed`
- `Timeout`
- `Shutdown`
- `Error`

## Active Forward Registry

Use the Lite runtime to query current forwards:

```go
forwards := rt.ActiveForwardsByClientIP(ip)
forward, ok := rt.ActiveForward(connectionID)
```

Returned values are snapshots (copies) and are safe for concurrent use.

## Limitations

Lite extensions do not expose the full Java proxy player lifecycle.

- No player objects
- No server switching APIs
- No proxy command APIs

This keeps Lite mode small and protocol-agnostic.
