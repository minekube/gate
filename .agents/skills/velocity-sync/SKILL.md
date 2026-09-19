---
name: velocity-sync
description: Review official PaperMC/Velocity changes for Gate ports when adding Minecraft Java protocol support or changing proxy behavior derived from Velocity.
---

# Velocity review for Gate

Before implementing a new Minecraft Java version or a Velocity-derived Gate behavior change, read [the sync record](references/VELOCITY_SYNC.md). Its verified sync point is evidence for one ported commit; its log records later reviews.

1. Resolve the current `PaperMC/Velocity@dev/3.0.0` head to a full SHA. Compare it against the newest reviewed head recorded in `references/VELOCITY_SYNC.md`; if that head is not pinned, compare from the verified sync commit and use the prior review to avoid repeating its decisions.
2. Inspect new commits for protocol IDs and packet layouts, login/configuration/play flow, forwarding, security, and proxy behavior that Gate implements. Read the actual diffs for plausible ports. Java API, Gradle, and release-only changes can be dismissed with a brief reason. For a new Minecraft version, cross-check the version and packet mappings against Velocity when available; if Velocity has not added that version, say so and use an authoritative protocol artifact for the implementation.
3. Port relevant behavior with regression coverage, or explain why each plausible change does not apply. Append a `sync` or `review` entry to `references/VELOCITY_SYNC.md` with an immutable compared head/range and the decision. Advance `verified_sync_point` only when a specific upstream commit demonstrably landed in Gate.
4. Keep the Gate PR title about the Gate change, without mentioning Velocity. Add only a short upstream reference in the body: the resolved PaperMC/Velocity head, compared range, and whether anything relevant was ported. Link the record update for details. If upstream has no new commits, say so for a new-version PR.

Do not claim general parity with Velocity. Follow the record's schema and run its focused tests after editing it.
