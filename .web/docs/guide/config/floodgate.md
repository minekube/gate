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
- Gate, when Trusted Floodgate is enabled, decrypts and validates this payload using your key.pem and extracts Bedrock identity (username, XUID, etc.).
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
- keyFile: path to Floodgate AES key (key.pem). This is the same file used by Floodgate/Geyser
- usernamePrefix: prefix added to Bedrock usernames (kept <=16 chars total)
- replaceSpaces: replace spaces in Bedrock usernames with underscores
- forceOffline: if true, Gate forces offline-mode for verified Bedrock sessions (skips Mojang auth)

Recommended Geyser/Floodgate setup
- Geyser config: `auth-type: floodgate`
- Place Floodgate key.pem in your Geyser data folder (or let Geyser auto-load from Floodgate)
- Ensure Floodgate is configured to forward data to your proxy/backend

Routing
- Run Geyser/Floodgate in front of Gate
- Point Bedrock clients to Geyser; Geyser connects to Gate
- Gate forwards Java connections to your backend servers as usual

Security notes
- Gate only trusts Floodgate payloads that decrypt and validate using your AES key
- Keep key.pem secure and consistent across Floodgate and Gate

Troubleshooting
- Invalid key: ensure Gate’s floodgate.keyFile points to the correct key.pem
- No Bedrock detection: verify that Geyser/Floodgate are injecting the Floodgate payload
- Username collisions: adjust usernamePrefix; ensure total username length <= 16

