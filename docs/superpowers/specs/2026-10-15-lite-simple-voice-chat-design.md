# Lite mode: Simple Voice Chat UDP relay

## Goal

Players connecting through Gate Lite can use the
[Simple Voice Chat](https://github.com/henkelmax/simple-voice-chat) mod. Voice
Chat runs over UDP next to the Minecraft TCP connection, and Lite only pipes
TCP today. Scope is Lite mode only; the full proxy modes are out of scope.

Target setup (confirmed with the requester): routes with **multiple backends**
and Lite strategies (round-robin, random, ...), so UDP must follow the backend
a player landed on via TCP.

## Non-goals

- Decrypting, parsing for content, or logging voice data.
- Voice Chat awareness in the full proxy, Bedrock, or Gate-side groups.
- Rewriting the `voicechat:secret` plugin message (backends advertise
  `voice_host` themselves, see Operator notes).

## Protocol facts (verified against Simple Voice Chat source/wiki)

- Client to server datagram: magic byte `0xFF`, player UUID (16 bytes,
  **plaintext**), then an AES-encrypted length-prefixed payload.
- Server to client datagram: magic byte, then the encrypted payload (no UUID).
- A server only decrypts and answers when it holds a secret for that UUID. An
  unknown UUID is dropped (or handed to the ping handler); a wrong secret fails
  decryption. The first client packet is `AuthenticatePacket` (0x5), answered
  by `AuthenticateAckPacket` (0x6).
- Server option `port` defaults to 24454/UDP; `-1` reuses the Minecraft port
  (the wiki discourages it). `voice_host` tells clients which host[:port] to use.

Not verified, must be checked during implementation against a real client:
which host the client uses when `voice_host` is empty.

## Design

### Config (per route, opt-in)

```yaml
lite:
  routes:
    - host: play.example.com
      backend: [10.0.0.1:25565, 10.0.0.2:25565]
      strategy: round-robin
      voiceChat:
        listen: ":24454"     # public UDP bind of Gate
        backendPort: 24454   # UDP voice port on the backends
```

`voiceChat` is a pointer; absent means no behavior change. Validation:
`listen` and `backendPort` must be valid ports; two routes may share a
`listen` only if they agree on `backendPort`. Lives in
`pkg/edition/java/lite/config`. No new warnings: `voiceChat` is a Lite-only
route field, so it is not part of `warnLiteIgnoredSettings`.

### Components (new package `pkg/edition/java/lite/voicechat`)

1. **Session table**: `Register(clientIP netip.Addr, backendHost string, backendPort int) (release func())`.
   Refcounted per (IP, backend). Called from `lite.Forward` right next to
   `strategyManager.TrackConnection`, using the TCP peer address after PROXY
   protocol handling. `release` runs on disconnect.
2. **Relay**: one UDP listener per distinct `listen`. For each client address
   (ip:port) it keeps a flow with an upstream UDP socket to the selected
   backend, copying both directions. Idle timeout (default 30s, refreshed by
   traffic in either direction), cap on concurrent flows (global, plus a
   per-client-IP limit, default 32), clean shutdown. Sessions are registered
   with the IP TCP actually dialed (`dst.RemoteAddr()`), so the relay does no
   DNS: registered backends are IP literals and anything unparseable is
   skipped. When a cap would refuse a new flow, the oldest unpinned
   (never answered) flow older than a 2s grace period is evicted instead
   (global cap: across all flows; per-IP cap: among that IP's flows); pinned
   flows are never evicted by cap pressure, and if nothing is evictable the
   new flow is refused.
3. **Backend selection for a new flow**:
   - IP has no registered session: drop silently (not an open relay).
   - One candidate backend: use it.
   - Several candidates (players behind one NAT on different backends): send
     the first datagram to every candidate; pin the flow to the first backend
     that replies, close the other upstream sockets. Unanswered flows expire
     with the idle timeout measured from creation (client traffic does not
     keep them alive) or as soon as the IP's backend set changes; the next
     datagram opens a fresh probing flow. This relies on backends ignoring packets of unknown
     UUIDs (see protocol facts).
4. **Lifecycle**: listeners are owned by the Lite instance and reconciled on
   config reload (start new, stop removed, restart on changed `listen`),
   using the existing route-change detection (`liteRoutesChanged` in
   `pkg/edition/java/proxy/proxy.go`). Closing a listener closes its flows.

### Data flow

Client TCP connects, Lite routes by host, picks backend B, `Register(clientIP, B)`.
Client receives `voicechat:secret` from the backend (unchanged, piped). Client
sends UDP to Gate `listen`; the relay maps it to B's voice port and relays
replies back from the same Gate socket.

### Error handling and safety

- Upstream dial/write errors close only that flow; the TCP session is untouched.
- Datagrams larger than 2048 bytes or without magic byte `0xFF` are dropped
  (cheap validation, no decryption).
- Never log payloads, UUIDs or player addresses above debug; follow the log
  privacy approach used for the Bedrock paths.
- Source IP is the real peer, never a client-supplied value.

### Operator notes (docs)

- Open the `listen` UDP port on Gate.
- On each backend set `voice_host` to Gate's public host[:port]; otherwise
  clients bypass the proxy.
- Backends must be reachable from Gate over UDP on `backendPort`.
- Limitation: relay mapping is by client IP, so one IP using two different
  Gate routes with voice chat enabled resolves via the probe above.

## Testing

- Unit: session table refcount/release; flow selection (none, one, many
  candidates); idle expiry; drop rules.
- Relay: loopback UDP sockets as fake clients and backends, including the
  multi-candidate probe pinning to the answering backend.
- Integration: two fake backends under `round-robin` (extend
  `integration_test.go` style), TCP connect per client, assert UDP reaches the
  backend chosen for that client.
- Config: validation cases and YAML round trip; shipped
  `pkg/configs/config-lite.yml` stays valid (gate_startup_test).
- Reload: removing a route's `voiceChat` closes its listener.

## Open items for implementation

- Confirm client fallback when `voice_host` is empty and whether `port=-1`
  setups need the same relay on the Minecraft port.
- Decide the concrete defaults (idle timeout, max flows) after a quick
  measurement; keep them overridable only if needed (YAGNI).
