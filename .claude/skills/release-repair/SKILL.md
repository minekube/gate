---
name: release-repair
description: Repair an already-published Gate release that has no assets by dispatching release-repair.yml without rewriting the historical release.
---

# Release repair

Releases already published with no assets are repaired by `.github/workflows/release-repair.yml` (manual dispatch from the default branch, contract pinned by `release_repair_workflow_test.go`). The two-job trust boundary is described in `.github/CLAUDE.md`.

## Procedure and gotchas

It must live on the default branch and check the tag out, because `workflow_dispatch` compiles the workflow file *at the dispatched ref* - a repair input added to `ci.yml` would not exist on the old tags that need it. The build job pins Go via `go-version-file: go.mod` *of the tag*: today's Go is not a neutral substitute for an old tag's, e.g. `go vet ./...` on v0.48.0 is clean at go1.24.1 and reports two printf diagnostics at go1.26.2, which `make test` would surface as a false red on source that shipped fine. GoReleaser runs with `--skip=publish` (assets are uploaded separately with `gh release upload --clobber`) so a repair never rewrites a historical release's notes or identity - older tags (v0.63.0 and back) predate `release: mode: keep-existing` and would otherwise be deleted and recreated. Tags whose `.goreleaser.yml` predates `version: 2` (v0.41.1 and older) cannot be repaired by a v2 toolchain; that is a finding to report, not a thing to work around.
