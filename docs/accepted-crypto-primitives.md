# Accepted SHA-1/MD5 usage (scanner triage)

Gate deliberately uses SHA-1 or MD5 in four places. Each one is fixed by an
external protocol or specification, so a scanner report against these sites is
**accepted as-is** and must not be "fixed" by swapping in a modern digest -
doing so would break interoperability or player identity. This document is the
rationale to cite when triaging such a report.

Scope note: this is a suppression list for these four sites only. Other
findings are triaged normally - the third finding in the 2026-09 report
("incorrect conversion between integer types") was a real bug
(the handshake port was decoded as a signed int16) and was fixed in
minekube/gate#1160.

| Site | Primitive | Why it is fixed |
| --- | --- | --- |
| `pkg/edition/java/auth/authenticator.go` `(*authenticator).GenerateServerID` | SHA-1 | The digest *is* the Mojang `hasJoined` `serverId` |
| `pkg/util/uuid/uuid.go` `OfflinePlayerUUID` | MD5 | It is the vanilla offline-mode UUIDv3 algorithm |
| `pkg/edition/bedrock/geyser/floodgate/floodgate.go` `JavaUuid` | SHA-1 | UUIDv5 is *defined* as SHA-1; the value is the Gate/Connect Bedrock identity contract |
| `pkg/internal/hashutil/util.go` `JsonHash` | SHA-1 | In-process change detection only; no security decision depends on it |

## 1. `GenerateServerID` - Mojang `hasJoined` serverId (SHA-1)

SHA-1 over the decrypted shared secret concatenated with the server's encoded
public key, rendered as vanilla renders it (hex, trimmed leading zeros, with
the two's-complement `-` prefix when the top bit is set).

The client computes the same digest from the same inputs and sends it to the
Mojang session server; the proxy then calls `hasJoined` with it. SHA-1 and the
signed-big-integer rendering are protocol constants: changing either makes the
session lookup fail and breaks online-mode login for every client.
Downgrading or upgrading this digest is not a local choice Gate is free to
make, and it protects nothing that a stronger digest would protect - the input
is a key exchange both sides already hold.

Cost of changing: online-mode authentication breaks outright.

## 2. `OfflinePlayerUUID` - vanilla offline UUIDv3 (MD5)

`MD5("OfflinePlayer:" + username)` with UUID version 3 and the RFC 4122
variant bits set. This is byte-for-byte the algorithm vanilla servers use for
players in offline mode (Bukkit's `UUID.nameUUIDFromBytes` produces the same
value).

The resulting UUID *is* the player's identity: whitelists, ops and ban lists,
world player data, and plugin-side stores are all keyed by it. A different
digest renames every offline-mode player and orphans their existing data.

Cost of changing: every offline-mode player gets a new identity, and existing
worlds/whitelists/plugins no longer match.

## 3. `JavaUuid` - Bedrock XUID mapping (UUIDv5)

A version 5 UUID over `"FloodgateXUID:" + XUID`. UUIDv5 is *defined* by
RFC 4122 as a SHA-1 based namespaced UUID, so "using SHA-1" and "generating a
v5 UUID" are the same statement.

The value is not Gate's private choice: it is the Bedrock XUID identity
contract shared across Minekube. Moxy derives the same UUID independently in
`connect/bedrockauth/xuid.go` (`DerivedUUIDForXUID`, same namespace, same v5
rule) and its admission check requires the forwarded profile UUID to equal
that derived value, so Gate and Moxy must agree byte for byte on the digest or
the same Bedrock player becomes two different Java identities.

Do not confuse this with the pre-existing `FloodgateJavaUuid` in the same file,
which is Floodgate's own `new UUID(0, xuid)` (first eight bytes zero, XUID in
the last eight) and involves no digest at all. Only `JavaUuid` is the
namespaced v5 derivation.

Cost of changing: Gate and Moxy (Connect) disagree about the Bedrock player's
Java UUID - forwarded profiles are rejected or a player's identity changes
across the two components.

## 4. `JsonHash` - in-process config fingerprint (SHA-1)

SHA-1 of the JSON encoding of a config struct, used by
`pkg/gate/api.go` (`Config.API`) and `pkg/gate/connect.go` (`Connect`) to
compare a new config against the previously seen one and skip a restart when
nothing changed.

The digest is stored in a local variable for the lifetime of the process, is
never persisted, transmitted, or compared against any external value, and no
integrity or authentication decision reads it. There is no security
consequence, and an attacker-controlled collision would only suppress a
service restart - the config content itself is what gets applied.

Cost of changing: none functionally; it is optional hygiene at most. If it is
ever changed, the algorithm must be swapped for *all* callers at once (the
comparison is only ever against hashes produced by the same function in the
same process).

## If a scanner (or a third party) reports one of these

1. Confirm the finding is at one of the four sites above (path + symbol).
2. Treat it as accepted: no code change, no algorithm change. The comment at
   each site points back to this document.
3. Suppress it at the scanner (GitHub code scanning dismissal with the reason
   "won't fix" and a link to this file) rather than weakening the scanner
   configuration or the check.
4. Anything not on this list - including the same primitives used elsewhere,
   for a new purpose - is a normal finding and should be triaged on its own
   merits, not dismissed by association with this list.
