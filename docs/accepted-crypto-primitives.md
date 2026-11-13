# Accepted scanner findings (protocol-mandated SHA-1/MD5 + the login RSA key default)

Gate deliberately uses SHA-1 or MD5 in four places, and deliberately generates a
1024-bit RSA key for the Java login handshake. Each one is fixed by an external
protocol or specification, or is a compatibility default the operator can raise,
so a scanner report against these sites is **accepted as-is** and must not be
"fixed" by swapping in a modern digest or by quietly changing the key size -
doing so would break interoperability or player identity. This document is the
rationale to cite when triaging such a report and the suppression list to apply
when the alert is dismissed - including the honest dismissal reason per site.

Scope note: this is a suppression list for these sites only. Other findings are
triaged normally - the third finding in the 2026-09 report
("incorrect conversion between integer types") was a real bug
(the handshake port was decoded as a signed int16) and was fixed in
minekube/gate#1160.

| Site | Primitive | Why it is fixed | Dismissal reason |
| --- | --- | --- | --- |
| `pkg/edition/java/auth/authenticator.go` `(*authenticator).GenerateServerID` | SHA-1 | The digest *is* the Mojang `hasJoined` `serverId` | `won't fix` |
| `pkg/util/uuid/uuid.go` `OfflinePlayerUUID` | MD5 | It is the vanilla offline-mode UUIDv3 algorithm | `won't fix` |
| `pkg/edition/bedrock/geyser/floodgate/floodgate.go` `JavaUuid` | SHA-1 | UUIDv5 is *defined* as SHA-1; the value is the Gate/Connect Bedrock identity contract | `won't fix` |
| `pkg/internal/hashutil/util.go` `JsonHash` | SHA-1 | In-process change detection only; no security decision depends on it | `false positive` |
| `pkg/edition/java/auth/authenticator.go` `privateKeyBits` / `New` (`DefaultPrivateKeyBits`) | 1024-bit RSA | Vanilla's login key size, operator-raisable via `auth.privateKeyBits` | `won't fix` |

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

## 5. `DefaultPrivateKeyBits` - the login RSA key (1024-bit default)

`New` generates the keypair Gate uses to encrypt the Java login handshake when
the caller supplies no key of its own:
`rsa.GenerateKey(rand.Reader, privateKeyBits(options))`, which resolves to
`DefaultPrivateKeyBits = 1024` unless the operator asked for another size.

1024 bits is the size vanilla Minecraft servers use for this key, and the size
BungeeCord and Velocity use. It is a deliberate compatibility default rather
than an oversight, and the key is the least dangerous place to keep one: it is
generated once per process start and never persisted, it encrypts the login
handshake of a single client, and it is discarded when the process exits - so
its exposure is session-scoped, and an attacker who could use RSA-1024
weakness against it would first have to already be on the wire of that one
login.

The default is operator-overridable. `auth.privateKeyBits` in `config.yml`
accepts 1024-8192, is validated at config load
(`config.MinPrivateKeyBits` / `config.MaxPrivateKeyBits`), and reaches the
generator through `auth.Options.PrivateKeyBits`. Unset (or `0`) keeps the
1024-bit default, so no existing deployment's crypto parameters change;
setting it takes effect on the next restart, because the key is generated once
at startup (Lite mode warns that it ignores the setting, since Lite forwards
the login to the backend). minekube/gate#1162 made that option effective and
lifted the login packet bounds that had pinned the handshake to a single
1024-bit RSA block, so a configured 2048/3072/4096/8192-bit key can now
complete a login.

Cost of changing the default: anything that assumes one 1024-bit RSA block on
the login handshake stops working - a login that used to succeed fails. That is
a compatibility change for existing deployments, not a hygiene cleanup, and it
is the only reason the default is not already higher.

### Why the default is not raised (the evidence the change needs)

Evidence already recorded (both from minekube/gate#1162):

- a probe driving the vanilla client's own JCA calls - `KeyFactory` +
  `X509EncodedKeySpec` on the DER key, `RSA/ECB/PKCS1Padding` for the
  ciphertexts - accepts Gate's 2048-bit key as well as its 1024-bit one: the
  2048-bit DER blob (294 bytes) parses, and its 256-byte ciphertexts decrypt
  through the `rsa.DecryptPKCS1v15` path Gate itself uses;
- a wire-level online-mode login through a real Gate listener completes with a
  2048-bit key, with the session-server stub observing the expected
  `sha1(sharedSecret ‖ serverPublicKey)` `serverId`
  (`TestOnlineModeLoginCompletesWithConfiguredKeyBits`, in-repo; 1024-bit
  control case alongside it).

Evidence still missing before the default can move:

- an end-to-end join with a **real** vanilla client (and a Bedrock client)
  against a Gate configured with 2048 bits - the probe above exercises the
  client's crypto calls, not its protocol implementation, so it cannot settle
  whether the client accepts the larger key in a live login;
- an audit of third-party tooling that parses or re-frames
  `EncryptionRequest` / `EncryptionResponse`. This is not hypothetical:
  upstream `PaperMC/Velocity` (`dev/3.0.0`) still reads the shared secret with
  a 128-byte bound and the public key with a 256-byte bound, i.e. code of that
  shape cannot carry a 2048-bit login key, and anything modelled on it inherits
  the same cap;
- an explicit decision to accept breaking any tool that turns out to have the
  1024-bit assumption, given the benefit is bounded by the key being ephemeral
  and session-scoped.

## If a scanner (or a third party) reports one of these

1. Confirm the finding is at one of the sites above (path + symbol, and the
   rule it came from - the two crypto rule titles seen so far are "Use of a
   broken or weak cryptographic hashing algorithm on sensitive data" and
   "Use of a weak cryptographic key").
2. Treat it as accepted: no code change, no algorithm change, no key-size
   change. The comment at each site points back to this document.
3. Dismiss the alert with the reason from the table above rather than weakening
   the scanner configuration or the check. For GitHub code scanning that is
   `PATCH /repos/{owner}/{repo}/code-scanning/alerts/{number}` with
   `{"state": "dismissed", "dismissed_reason": "<reason>", "dismissed_comment": "<link to this file>"}`;
   `dismissed_reason` accepts four values - `false positive`, `won't fix`,
   `used in tests`, `mitigated` (GitHub's REST schema). Each site's reason has to
   be the honest one: `won't fix` means the alert is real but the change is one
   we have decided not to make, `false positive` means the alert's premise does
   not hold here. `used in tests` is not applicable to any site above, because
   all five are production code, and `mitigated` is not used either: none of
   these sites has a control that removes the weakness, and claiming one would
   make the record harder to defend rather than easier. `JsonHash` is the one
   `false positive`: the rule there is about hashing *sensitive data*, and the
   digest is an in-process change fingerprint with no security consequence.
4. Anything not on this list - including the same primitives used elsewhere,
   for a new purpose - is a normal finding and should be triaged on its own
   merits, not dismissed by association with this list.

Note on the enablement state: this repository does not have GitHub code
scanning configured today, so there are no alert IDs to dismiss and no
dismissals have been recorded. The reasons above are the mapping to apply when
it is enabled and these sites are reported.
