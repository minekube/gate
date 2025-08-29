---
title: Trusted Floodgate (Geyser) Bedrock Support
outline: deep
---

This page describes how to run Geyser + Floodgate in front of Gate, and let Gate trust Floodgate-authenticated Bedrock players without double authentication.

Prerequisites
- GeyserMC + Floodgate running on your edge (Velocity/Spigot/Bungee) or standalone Geyser
- The Floodgate AES key file (key.pem) available to Gate

Why
- Avoid implementing translation in Gate
- Let Bedrock players join without Java Edition accounts, authenticated via Floodgate
- Keep resource usage minimal by running Geyser/Floodgate in front

How it works
- Floodgate encrypts BedrockData using AES-GCM and injects it into the Java handshake hostname as a string starting with `^Floodgate^`.
- Gate decrypts and validates this payload using your key.pem and extracts Bedrock identity (username, XUID, etc.).
- If verification succeeds, Gate strips the payload from the hostname, constructs a Java-compatible username, and forces offline-mode for that session (skips Mojang auth).

Configuration
In your Gate config (config.yml):

```yaml
floodgate:
  enabled: true
  keyFile: /path/to/key.pem
  usernamePrefix: "."
  replaceSpaces: true
  forceOffline: true
```

- enabled: turn on trusted Floodgate mode
- keyFile: path to Floodgate AES key (key.pem)
- usernamePrefix: prefix added to Bedrock usernames (kept <=16 chars total)
- replaceSpaces: replace spaces in Bedrock usernames with underscores
- forceOffline: if true, Gate forces offline-mode for verified Bedrock sessions (skips Mojang auth)

Built-in plugin mode
- Gate compiles a built-in plugin that adjusts the Java `GameProfile` (name/UUID) when Floodgate is enabled.
- You don't need to install external modules; simply enable `floodgate.enabled` and provide `keyFile`.

Example: Geyser+Floodgate → Gate
- Run Geyser with Floodgate and point it at Gate's Java bind
- Gate config: enable Floodgate (see above)
- Connect a Bedrock client to Geyser and verify join without Mojang auth; username will include your prefix

Security notes
- Gate only trusts Floodgate payloads that decrypt and validate using your AES key
- Keep key.pem secure and consistent across Floodgate and Gate

Troubleshooting
- Invalid key: ensure Gate’s `floodgate.keyFile` points to the correct `key.pem`
- No Bedrock detection: verify that Geyser/Floodgate are injecting the Floodgate payload
- Username collisions: adjust `usernamePrefix`; ensure total username length <= 16

