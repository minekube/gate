# Velocity sync record

Gate ports behavior from [PaperMC/Velocity](https://github.com/PaperMC/Velocity). This file is the
committed record of **which upstream commit we have positively verified as landed in Gate**, plus a
log of every upstream review since. It exists because that answer previously had to be reconstructed
by archaeology across branch names, commit contents and upstream dates.

If you sync from Velocity, or review an upstream range and decide nothing is portable, **append an
entry to the log below**. That is the whole maintenance burden.

## What this file claims — and what it does not

Read this before quoting the file anywhere.

**It claims:** upstream commit `a7581821` was ported into Gate as `48a7f91`, merged via PR #781 on
2026-06-16. That is a verified, evidenced fact — the Gate commit implements that upstream commit's
behavior (a shared per-proxy session ID, regenerated only once the proxy empties) and is pinned by
`pkg/edition/java/proxy/session_id_test.go`.

**It does not claim** that every upstream commit *before* `a7581821` was ported. Nothing in this
repository establishes that, and no record exists that could. The verification is per-commit, not
cumulative.

This is why the field below is called a **verified sync point as of** a commit — and never a claim of
general parity with upstream as of that commit. Those two sentences look alike and mean completely
different things. The second is the kind of claim that is true when written and quietly becomes a lie
nobody notices, so do not introduce it, or any paraphrase of it, anywhere in this file. Say what was
verified; say plainly what was not. The log is designed to grow forward from a point we can actually
stand behind, so coverage becomes provable going forward rather than asserted retroactively.

`velocity_sync_test.go` enforces this with an exact-match blocklist of phrasings. The banned phrasings
are deliberately spelled out in the test rather than here, so this file never contains the sentence it
forbids.

## Verified sync point as of

<!-- Machine-readable record. Parsed by TestVelocitySyncRecord* in velocity_sync_test.go.
     Keep this the first ```yaml block in the file. -->

```yaml
verified_sync_point:
  upstream_repo: PaperMC/Velocity
  upstream_branch: dev/3.0.0
  upstream_commit: a7581821fb72a3eb5011f725d8876c91aa7843e1
  upstream_subject: Use a shared per-proxy session ID for 26.2 login metrics
  upstream_date: 2026-06-16
  gate_commit: 48a7f910bdc2e7143294dedea326b445597226a9
  gate_merge_commit: 7496e3f0665c49f9d3f9045fe4d809ada7728893
  gate_pr: 781
  gate_date: 2026-06-16
  verified_on: 2026-07-28

log:
  - date: 2026-06-16
    kind: sync
    upstream_commit: a7581821fb72a3eb5011f725d8876c91aa7843e1
    gate_commit: 48a7f910bdc2e7143294dedea326b445597226a9
    gate_pr: 781
    summary: >-
      Ported upstream a7581821 (shared per-proxy session ID for 26.2 login metrics) along with
      Minecraft 26.2 protocol support. Landed on branch codex/sync-velocity-pr-14. This commit is
      also exactly the merge base of robinbraemer/Velocity@dev/3.0.0 with PaperMC/Velocity@dev/3.0.0.

  - date: 2026-07-28
    kind: review
    upstream_range: a7581821fb72a3eb5011f725d8876c91aa7843e1..PaperMC/Velocity@dev/3.0.0
    upstream_commit_count: 11
    ported: none
    summary: >-
      Reviewed all 11 upstream commits (8 files) individually; none carried behavior portable to Gate.
      Content was Gradle build/dependency churn, the Adventure 4->5 Java API migration
      (ConnectedPlayer/VelocityConsole sendMessage overloads, FacetPointers removal), a JLine console
      history variable and a Netty DNS resolver thread-pool fix — all constructs Gate has no
      counterpart for — plus release plumbing. The one apparent behavior change, removal of
      .downsampleColors() from PRE_1_16_SERIALIZER, was verified a no-op: Adventure 5 deleted that
      builder method, upstream already sets the same flag via JSONOptions.EMIT_RGB=FALSE, and Gate
      downsamples at the same <1.16 boundary via codec.JsonPre1_16 (NoDownsampleColor: false). No Gate
      change was made, so the verified sync point above is unchanged by this review.
```

## The log

The `log` list answers the question a single SHA cannot: **"was upstream commit X ever considered?"**
A `sync` entry records upstream work that was ported. A `review` entry records an upstream range that
was examined and deliberately not ported, with the reason — a reviewed-and-rejected commit is a
different state from an unseen one, and only the log distinguishes them.

### Appending an entry

Add a new item to the end of `log`, newest last. Fields:

| Field | Applies to | Meaning |
|---|---|---|
| `date` | both | ISO `YYYY-MM-DD` of the sync or review |
| `kind` | both | `sync` or `review` |
| `summary` | both | What was ported, or what was reviewed and why it was not ported |
| `upstream_commit` | `sync` | Full 40-hex upstream SHA that was ported |
| `gate_commit` | `sync` | Full 40-hex Gate SHA that landed it |
| `gate_pr` | `sync` | Gate PR number |
| `upstream_range` | `review` | The upstream range examined |
| `upstream_commit_count` | `review` | How many commits the range held |
| `ported` | `review` | `none`, or what was taken from the range |

When an entry is a `sync` that advances the verified point, **also update `verified_sync_point`** to
that commit pair and set `verified_on` to the date you confirmed it. Verify before you write it: the
Gate commit should demonstrably implement the upstream commit's behavior, ideally with a test.

### Reviewing an upstream range

`robinbraemer/Velocity` is the fork; `PaperMC/Velocity@dev/3.0.0` is upstream. A GitHub compare
between them gives the outstanding range.

One filter is worth knowing up front: Gate does not track Adventure's **Java API**. It uses its own Go
stack (`go.minekube.com/common/minecraft/component`), kept aligned with Adventure by hand at the
**wire format** level only — see the codec presets and their "Equivalent to ..." comments in
`pkg/edition/java/proto/util/chat.go`. Adventure builder/overload/type churn is therefore permanently
non-portable and can be filtered out on sight; only wire-format and protocol changes matter.

## Related

- `AGENTS.md` — Gate's project agent memory, which points here.
- `velocity_sync_test.go` — the guard over this file. It checks shape, not truth; read its header
  comment for exactly what it does and does not prove.
