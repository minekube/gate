## Summary

Trusted Floodgate (Geyser) integration for Bedrock support without Java accounts:

- Add `config.floodgate` (enabled, keyFile, usernamePrefix, replaceSpaces, forceOffline)
- Detect and decrypt Floodgate hostname payload (AES-GCM with Base64 topping, `^Floodgate^` header)
- Strip payload, build Java username, and force offline-mode for verified sessions to avoid double auth
- Documentation under `.web/docs/guide/config/floodgate.md`

## Motivation

Allow Bedrock players (via Geyser/Floodgate) to connect through Gate efficiently without implementing protocol translation in Gate and without requiring Java accounts.

## Implementation Notes

- Decryption mirrors Floodgate's AesCipher: AES/GCM/NoPadding with 12-byte IV and Base64 topping (IV + 0x21 + ciphertext)
- Payload extracted from handshake hostname split by `\0`
- Verified trust is propagated from handshake to login phase; login forced offline if `forceOffline=true`

## Testing

- Unit: `pkg/edition/java/proxy/floodgate_test.go` validates detect/decrypt/clean and username build
- Manual/E2E recommended:
  - Run Geyser+Floodgate in front of Gate
  - Configure Gate with `floodgate.enabled=true` and matching `keyFile`
  - Connect a Bedrock client and verify join without Mojang auth

## Docs

- `.web/docs/guide/config/floodgate.md`

## Backwards Compatibility

- Disabled by default; no behavior change unless `floodgate.enabled=true`

