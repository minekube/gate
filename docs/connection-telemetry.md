# Connection telemetry contract

Gate exports the bounded, privacy-safe metric contract shared with Moxy and
Minekube monitoring:

- `gate_connection_events_total{protocol,connection_kind,stage,outcome}`
- `gate_network_bytes_total{boundary,protocol,connection_kind,direction,stage}`
- `gate_connection_duration_seconds{protocol,connection_kind,outcome}`
- `gate_active_connections{protocol,connection_kind,stage}`

`protocol` is `java` or `bedrock`; `connection_kind` is `unknown`, `status`,
`login`, `transfer`, or `gameplay`; `direction` is `rx` or `tx`; and boundary
is `client_edge`, `connector_tunnel`, `bedrock_loopback`, or `backend`. Stages
and outcomes are closed enums in `pkg/telemetry/connection`.

The Java front door attaches exactly one raw-socket counter after connection
event replacement. Lite's two `io.Copy` directions therefore contribute through
that same client wrapper; route lookup, backend dial, and pipe failures only
advance the bounded lifecycle outcome. Geyser's in-process handoff is explicitly
marked `bedrock_loopback`, preserving one session rather than double counting.

Gate sends the same schema to its configured OTel meter and to the process-wide
Prometheus registry. Moxy's private health server exposes that registry at
`0.0.0.0:8086/metrics`; `deploy/fly/fly.toml` declares this exact endpoint for
Fly scraping. GitOps federates only the four families above. No metric label or
event field can contain an address, host, port, endpoint, session/player ID,
XUID, packet name, or error text.
