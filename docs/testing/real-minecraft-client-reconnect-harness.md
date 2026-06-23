# Real Minecraft Client Reconnect Harness

Source issue: MIN-24.

## Purpose

Gate reconnect and configuration-phase bugs can sit below the level covered by unit tests and synthetic packet stubs. The harness for this bug class must prove the user-visible failure path with a real Minecraft client implementation before production code is changed.

The current Gate test suite already has useful wire-level coverage, including Forge login relay and configuration packet handling. That is not enough for the Poorpur-style reconnect/configuration issue class because the failure depends on the client state machine around play -> configuration -> play transitions, reconnect timing, and how the client reacts when the proxy and backend disagree about state.

## Valid Client Tiers

Tier 1 is the default acceptance target: an official Minecraft Java client launched in an isolated test profile, driven through a local proxy/backend fixture, with logs and packet captures collected as test artifacts. This is the strongest match for reported player behavior.

Tier 2 is acceptable only when Tier 1 is impractical for CI: a headless integration client library that implements the same Minecraft Java protocol state machine and is pinned to the affected protocol version. The first Tier 2 implementation must be calibrated against one local Tier 1 run for the same scenario, using packet trace checkpoints and client log evidence.

Tier 3 is not sufficient for closing this issue class: Go-only fake clients, hand-written packet scripts, or tests that only assert individual packet codecs. These remain useful regression tests after a real-client reproduction exists, but they do not establish that the player-facing bug is reproduced.

## Red/Green Rule

1. Create or select a real-client scenario that fails against the current Gate revision.
2. Capture the red evidence: command, Gate commit, Minecraft version, backend fixture version, client log excerpt, packet trace checkpoints, and the final assertion that failed.
3. Implement the smallest Gate fix.
4. Re-run the same scenario and record green evidence with the same fields.
5. Add a cheaper regression test only after the real-client scenario has proven the failure mode.

No production reconnect/configuration fix should be merged for this class unless step 1 is automated or the CTO accepts a documented manual reproduction script as a temporary bridge.

## First Test Target

Target: a Minecraft 1.20.2+ client connected through Gate to a local backend fixture that forces a backend play -> configuration transition and then disconnects or rejects during reconfiguration.

Why 1.20.2+: the configuration state is part of the client protocol path. Gate code paths currently involved include:

- `backendPlaySessionHandler.handleStartUpdate`: backend sends `StartUpdate`; Gate sets backend reader to config and calls `connectedPlayer.switchToConfigState`.
- `connectedPlayer.switchToConfigState`: Gate sends client `StartUpdate`, sets `pendingConfigurationSwitch`, switches writer state to config, enables the play packet queue, and flushes.
- `clientPlaySessionHandler.handleFinishedUpdate`: client responds with `FinishedUpdate`; Gate expects `pendingConfigurationSwitch`, switches the client session handler to config, forwards `FinishedUpdate` to the backend, and completes the config switch future.
- `backendTransitionSessionHandler.handleJoinGame`: when switching back to play on 1.20.2+, Gate waits for the client play session handler after configuration completes.

Red assertion: the harness fails if the client is disconnected, hangs past a fixed timeout, or logs a known configuration/reconnect failure before reaching a stable play state after the forced reconfiguration.

Green assertion: the same client reaches stable play state after the forced reconfiguration, and Gate logs show the expected play -> config -> play transition without an unexpected "not expecting reconfiguration" path.

## Fixture Shape

The fixture should run entirely on loopback:

- Gate under test, built from the checked-out commit.
- A deterministic backend fixture that can script:
  - initial login and play join;
  - `StartUpdate` while in play;
  - optional plugin messages, registry sync, known packs, and `FinishedUpdate`;
  - forced disconnect or reconnect trigger at a named point.
- A client runner that starts a clean Minecraft profile, joins the Gate listener, waits for a stable play indicator, and exits.
- Artifact capture:
  - Gate logs;
  - backend fixture logs;
  - client `latest.log`;
  - packet trace summary, not full private player data;
  - junit-style pass/fail result.

## CI Requirements

The first automated lane should be opt-in because official client automation needs heavier dependencies and Mojang/EULA-sensitive handling:

- Linux runner with Java and an isolated Minecraft cache.
- No user Microsoft/Mojang account credentials.
- Offline/local-only server path.
- Version-pinned client installation cache.
- Time-boxed run, expected under five minutes after cache warmup.
- Artifacts retained on failure.

Once stable, add a scheduled or label-gated GitHub Actions workflow rather than putting the full official-client lane into every pull request. Keep fast Go unit and wire tests in regular CI.

## Secrets And Access

Required for implementation:

- GitHub contents and pull request write access for Gate branches and PRs.
- GitHub Actions workflow write access only if the first implementation adds or changes workflow files.
- Permission to cache public Minecraft client artifacts in CI if the chosen official-client runner supports that legally and operationally.

Not required:

- Production Gate, Connect, or Fly mutations.
- User Minecraft accounts.
- Raw Discord/support exports in repository tests.
- Production player identifiers, IP addresses, or session tokens.

## Privacy And Safety Boundaries

The harness must use synthetic usernames, loopback addresses, generated UUIDs, and local-only servers. Do not commit production logs, raw support transcripts, IP addresses, access tokens, or player account data. If a support report is used to choose the scenario, reduce it to the technical shape: client version, backend family, transition point, observed disconnect message, and expected behavior.

## Implementation Path

1. Add a small `realclient` test package or external harness directory that owns process orchestration and artifacts.
2. Implement the backend script for the first play -> config -> play or play -> config -> disconnect scenario.
3. Implement the client runner using Tier 1 official-client automation if feasible; otherwise use a Tier 2 headless client and record the required calibration gap.
4. Add a local command such as `make test-real-client-reconnect` that fails fast when required tools are missing and writes artifacts to a deterministic directory.
5. Add the first red test against current Gate, then fix the Gate reconnect/configuration behavior in a separate PR or commit stack.

## Current Blockers

This design can be reviewed now. Executable implementation needs a runtime with Go and Java tooling. The current Paperclip container used for MIN-24 has neither `go` nor `java` available, so it cannot produce or verify the real-client executable lane in this heartbeat.
