#!/usr/bin/env bash
# Run golangci-lint the way Gate needs it run: after a warm, complete build
# cache, from a clean linter cache, and with a bounded retry for the one
# failure class that is not a finding.
#
# Why this wrapper exists (2026-11-13)
# ------------------------------------
# `golangci-lint run --timeout 3m0s` (v1.64.8) intermittently never reaches
# analysis on GitHub's runners and reports nothing about the code:
#
#   level=warning msg="[runner] Can't run linter goanalysis_metalinter:
#     buildir: failed to load package : could not load export data:
#     no export data for \"golang.org/x/image/math/f32\""
#   level=error msg="Running error: can't run linter goanalysis_metalinter ..."
#   ##[error]Process completed with exit code 3.
#     — run 43448862215 (PR #1337), job 131965443742
#
# Its sibling shape is a burst of bogus typecheck diagnostics against code that
# compiles: 51 lines of "type *connectedPlayer has no field or method Type
# (typecheck)" in 44 seconds on a tree whose `test` jobs were green (release
# gate run 43448561385, job 131964515714). Both shapes are the linter failing
# to load the same package graph, not a verdict, and a plain retry of the same
# tree came back clean (run 43448561447) — which is why the durable behaviour
# below is "retry only this class, then fail closed", never "ignore the linter".
#
# There is a second, release-specific reason for the warm build: a flake in the
# `trigger-release / lint` job of release-publish.yml skips release-build,
# publish-images and publish-release for that tag, i.e. a silent no-release
# (v0.79.1 shipped 0 assets until a re-dispatch landed). Retrying inside the
# step keeps one flake from skipping a publish without weakening the gate: a
# real finding still fails the step, after the retry.
#
# The classification, in order:
#   * exit 0                                   -> success, stop.
#   * exit 3                                   -> golangci-lint could not run
#                                                 (its "Failure" code; findings
#                                                 are exit 1) -> retry.
#   * loader signature in the output           -> retry.
#   * exit 1 where EVERY diagnostic carries the
#     "(typecheck)" analyser suffix            -> the tree was already proven to
#                                                 compile by `go build ./...`
#                                                 above, so this is the same
#                                                 broken load -> retry.
#   * anything else                            -> report it and fail immediately.
# After the last attempt the step fails with the linter's own exit code.
set -euo pipefail

SELF="${BASH_SOURCE[0]}"
LINT_TIMEOUT="${LINT_TIMEOUT:-3m0s}"
LINT_ATTEMPTS="${LINT_ATTEMPTS:-3}"
LINT_RETRY_DELAY="${LINT_RETRY_DELAY:-5}"
LINT_BIN="${LINT_BIN:-golangci-lint}"
GO_BIN="${GO_BIN:-go}"

# The loader-failure signatures observed on this repository (see the header).
LOADER_PATTERN='could not load export data|can.t run linter goanalysis_metalinter|buildir: failed to load package'

log() { printf 'run-golangci-lint: %s\n' "$*" >&2; }

# is_loader_failure <output-file>
is_loader_failure() {
  grep -Eq "$LOADER_PATTERN" "$1"
}

# is_typecheck_only <output-file> — true when the run reported at least one
# diagnostic and every one of them is a "(typecheck)" line.
is_typecheck_only() {
  local diagnostics
  diagnostics="$(grep -E '^[^[:space:]]+\.go:[0-9]+:[0-9]+: ' "$1" || true)"
  [ -n "$diagnostics" ] || return 1
  if printf '%s\n' "$diagnostics" | grep -Evq '\(typecheck\)[[:space:]]*$'; then
    return 1
  fi
  return 0
}

lint_once() { # $1 = output file -> golangci-lint's exit code (findings = 1)
  local out="$1" code=0
  set +e
  "$LINT_BIN" run --timeout "$LINT_TIMEOUT" 2>&1 | tee "$out"
  code="${PIPESTATUS[0]}"
  set -e
  return "$code"
}

main() {
  log "$("$LINT_BIN" --version 2>&1 | head -n1)"
  log "$("$GO_BIN" version 2>&1)"

  # 1. Warm the build cache. This both proves the tree compiles (a compiler
  #    error is a real failure and must be reported as one) and leaves the
  #    complete export-data set the linter loads, instead of racing the
  #    half-populated module/build cache a cold runner starts with.
  log "warming the build cache: $GO_BIN build ./..."
  local build_out=""
  if ! build_out="$("$GO_BIN" build ./... 2>&1)"; then
    printf '%s\n' "$build_out"
    log "the tree does not build; failing on the compiler output instead of a linter verdict"
    return 1
  fi
  [ -n "$build_out" ] && printf '%s\n' "$build_out"

  # 2. Start from a clean linter cache: a partially written cache is the other
  #    half of this failure class. Best effort — if the cache cannot be cleaned
  #    the lint run below still decides, so a cleaning failure is not fatal.
  if ! "$LINT_BIN" cache clean >/dev/null 2>&1; then
    log "warning: '$LINT_BIN cache clean' failed; continuing with the existing cache"
  fi

  local attempt=1 code=0 out="" reason=""
  while [ "$attempt" -le "$LINT_ATTEMPTS" ]; do
    out="$(mktemp)"
    log "attempt $attempt/$LINT_ATTEMPTS: $LINT_BIN run --timeout $LINT_TIMEOUT"
    code=0
    lint_once "$out" || code=$?

    if [ "$code" -eq 0 ]; then
      rm -f "$out"
      log "attempt $attempt: clean"
      return 0
    fi

    reason=""
    if is_loader_failure "$out"; then
      reason="package-loader failure (export data / goanalysis_metalinter)"
    elif [ "$code" -eq 3 ]; then
      reason="golangci-lint could not run (exit 3 means a failure, not findings)"
    elif [ "$code" -eq 1 ] && is_typecheck_only "$out"; then
      reason="typecheck-only report on a tree that compiles (loader inconsistency)"
    fi
    rm -f "$out"

    if [ -z "$reason" ]; then
      log "attempt $attempt: golangci-lint reported findings (exit $code); not retrying"
      return "$code"
    fi

    log "attempt $attempt: $reason"
    if [ "$attempt" -lt "$LINT_ATTEMPTS" ]; then
      log "retrying after ${LINT_RETRY_DELAY}s"
      sleep "$LINT_RETRY_DELAY"
      attempt=$((attempt + 1))
      continue
    fi
    log "all $LINT_ATTEMPTS attempts failed with a non-deterministic linter error; failing closed (exit $code)"
    return "$code"
  done
}

# ---------------------------------------------------------------------------
# hermetic self-test: stub `go` and `golangci-lint` on PATH and drive every
# decision branch, so a change to the classification fails CI instead of
# silently turning a finding into a retry or a flake into a red release gate.
# ---------------------------------------------------------------------------
self_test() {
  local tmp; tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  local cases=0 bad=0

  stub_go() { # $1 = bin dir
    cat > "$1/go" <<'STUB'
#!/usr/bin/env bash
printf 'go %s\n' "$*" >> "$STUB_STATE/calls"
if [ "${1:-}" = build ] && [ "${STUB_BUILD_EXIT:-0}" != "0" ]; then
  echo "pkg/broken/broken.go:3:2: undefined: nope" >&2
  exit "$STUB_BUILD_EXIT"
fi
exit 0
STUB
    chmod +x "$1/go"
  }

  stub_lint() { # $1 = bin dir
    cat > "$1/golangci-lint" <<'STUB'
#!/usr/bin/env bash
case "${1:-}" in
  --version) echo "golangci-lint has version v0.0.0-self-test"; exit 0 ;;
  cache) printf 'lint cache clean\n' >> "$STUB_STATE/calls"; exit 0 ;;
  run) ;;
  *) echo "self-test stub: unexpected arguments: $*" >&2; exit 99 ;;
esac
printf 'lint run\n' >> "$STUB_STATE/calls"
entry="$(head -n1 "$STUB_STATE/queue")"
if [ -n "$entry" ]; then
  tail -n +2 "$STUB_STATE/queue" > "$STUB_STATE/queue.next"
  mv "$STUB_STATE/queue.next" "$STUB_STATE/queue"
fi
code="${entry%%:*}"
fixture="${entry#*:}"
if [ -n "$fixture" ] && [ "$fixture" != "$entry" ]; then
  cat "$STUB_STATE/$fixture"
fi
exit "${code:-0}"
STUB
    chmod +x "$1/golangci-lint"
  }

  write_fixtures() { # $1 = state dir
    : > "$1/clean.txt"
    cat > "$1/loader.txt" <<'TXT'
level=warning msg="[runner] Can't run linter goanalysis_metalinter: buildir: failed to load package : could not load export data: no export data for \"golang.org/x/image/math/f32\""
level=error msg="Running error: can't run linter goanalysis_metalinter\nbuildir: failed to load package : could not load export data: no export data for \"golang.org/x/image/math/f32\""
TXT
    cat > "$1/typecheck.txt" <<'TXT'
pkg/edition/java/proxy/server.go:449:21: s.player.Type undefined (type *connectedPlayer has no field or method Type) (typecheck)
pkg/util/connectutil/single/single.go:211:14: s.Propose undefined (type *server has no field or method Propose) (typecheck)
TXT
    cat > "$1/finding.txt" <<'TXT'
pkg/gate/gate.go:42:2: this value of err is never used (ineffassign)
TXT
    cat > "$1/mixed.txt" <<'TXT'
pkg/util/connectutil/single/single.go:211:14: s.Propose undefined (type *server has no field or method Propose) (typecheck)
pkg/gate/gate.go:42:2: this value of err is never used (ineffassign)
TXT
  }

  expect_case() { # $1 name, $2 want exit, $3 queue ("code:fixture" per attempt), $4 want lint runs, rest = needles ("!" prefix = must be absent)
    local name="$1" want="$2" queue="$3" want_runs="$4"
    shift 4
    local dir="$tmp/$name" out code got_runs ok=1 needle
    mkdir -p "$dir/bin" "$dir/state"
    stub_go "$dir/bin"
    stub_lint "$dir/bin"
    write_fixtures "$dir/state"
    printf '%s\n' $queue > "$dir/state/queue"
    : > "$dir/state/calls"

    out="$(PATH="$dir/bin:$PATH" STUB_STATE="$dir/state" STUB_BUILD_EXIT="${CASE_BUILD_EXIT:-0}" \
      LINT_RETRY_DELAY=0 LINT_ATTEMPTS="${CASE_ATTEMPTS:-3}" bash "$SELF" 2>&1)" || code=$?
    code="${code:-0}"
    cases=$((cases + 1))

    if [ "$code" != "$want" ]; then
      printf '  exit %s, want %s\n' "$code" "$want"
      ok=0
    fi
    got_runs="$(grep -c '^lint run$' "$dir/state/calls" 2>/dev/null || true)"
    got_runs="${got_runs:-0}"
    if [ "$got_runs" != "$want_runs" ]; then
      printf '  lint runs %s, want %s\n' "$got_runs" "$want_runs"
      ok=0
    fi
    for needle in "$@"; do
      case "$needle" in
        "!"*)
          if [[ $out == *"${needle#!}"* ]]; then printf '  unexpected output: %s\n' "${needle#!}"; ok=0; fi
          ;;
        "calls:"*)
          if ! grep -Fqx "${needle#calls:}" "$dir/state/calls"; then printf '  missing call: %s\n' "${needle#calls:}"; ok=0; fi
          ;;
        *)
          if [[ $out != *"$needle"* ]]; then printf '  missing output: %s\n' "$needle"; ok=0; fi
          ;;
      esac
    done
    if [ "$ok" = 1 ]; then
      printf 'PASS %s\n' "$name"
    else
      printf 'FAIL %s\n%s\n' "$name" "$out"
      bad=$((bad + 1))
    fi
  }

  # 1. A clean first attempt: warm build + cache clean + exactly one lint run.
  expect_case clean-first-try 0 "0:clean.txt" 1 \
    "attempt 1: clean" "calls:go build ./..." "calls:lint cache clean"

  # 2. The export-data flake, then a clean retry: the release gate case.
  expect_case loader-flake-retried 0 "3:loader.txt 0:clean.txt" 2 \
    "attempt 1: package-loader failure" "attempt 2: clean"

  # 3. The typecheck-cascade shape of the same broken load.
  expect_case typecheck-cascade-retried 0 "1:typecheck.txt 0:clean.txt" 2 \
    "attempt 1: typecheck-only report on a tree that compiles" "attempt 2: clean"

  # 4. A real finding must never be retried away.
  expect_case finding-not-retried 1 "1:finding.txt" 1 \
    "never used (ineffassign)" "not retrying" "!attempt 2"

  # 5. A typecheck diagnostic next to a real finding is a finding.
  expect_case mixed-findings-not-retried 1 "1:mixed.txt" 1 \
    "not retrying" "!attempt 2"

  # 6. A flake on every attempt fails closed with the linter's exit code.
  expect_case flake-exhausted-fails-closed 3 "3:loader.txt 3:loader.txt 3:loader.txt" 3 \
    "failing closed" "attempt 3/3"

  # 7. A tree that does not compile fails on the compiler output, before lint.
  CASE_BUILD_EXIT=1 expect_case build-error-fails-closed 1 "0:clean.txt" 0 \
    "does not build" "undefined: nope"
  CASE_BUILD_EXIT=0

  # 8. One retry only, when configured that way, still fails closed.
  CASE_ATTEMPTS=2 expect_case two-attempts-then-fail-closed 3 "3:loader.txt 3:loader.txt" 2 \
    "failing closed" "attempt 2/2"
  CASE_ATTEMPTS=3

  printf 'run-golangci-lint self-test: %d case(s), %d failure(s)\n' "$cases" "$bad"
  [ "$bad" -eq 0 ]
}

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

main "$@"
