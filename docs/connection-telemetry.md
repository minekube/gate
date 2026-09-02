# Connection telemetry contract

Gate exports `gate.connection.events` and `gate.connection.bytes` as the
versioned `gate.connection.v1` consumer contract. Every event has only these
bounded dimensions: `gate.connection.schema`, `gate.connection.kind`,
`gate.connection.stage`, `gate.connection.outcome`; byte observations add the
bounded `gate.connection.direction` (`read` or `write`).

Kinds are `status`, `login`, `transfer`, `gameplay`, and `unknown`. Stages are
`accepted`, `handshake`, `auth`, `backend`, `play`, and `closed`. Outcomes are
`unknown`, `success`, `failed`, `timeout`, `rate_limited`, `backend_failed`,
and `closed`.

This is an export-only Gate contract: it has no Moxy runtime dependency. In
particular it never emits addresses, hosts, ports, endpoints, session or player
identifiers, XUIDs, packet labels, or error text.
