#!/usr/bin/env bash
#
# Guard: a tool pinned in a workflow step (`go run <pkg>@<version>`,
# `go install <pkg>@<version>`) must be buildable with the Go toolchain that
# step actually runs with.
#
# Why this exists: on 2026-09-19 `gohawk@v0.3.2` raised its `go` directive to
# 1.27 while the lint job installed go1.26.0 from go.mod and ran with the
# runner's GOTOOLCHAIN=local. Every branch went red before the linter did any
# work:
#   go: github.com/kojah/gohawk@v0.3.2: requires go >= 1.27.0
#       (running go 1.26.0; GOTOOLCHAIN=local)
# which blocked every PR that waits on ci.yml (incl. the release cascade's
# bump PRs). See minekube/gate#1133.
#
# For every pin found in the workflow files this script resolves the module's
# `go` directive from the module proxy and compares it with the toolchain of
# the enclosing step, resolved in this order:
#   1. the step's own GOTOOLCHAIN (env key or inline `GOTOOLCHAIN=... `)
#   2. the job's env GOTOOLCHAIN
#   3. the workflow's env GOTOOLCHAIN
#   4. the job's `actions/setup-go` version (`with: go-version:`, or the `go`
#      directive of the file named by `with: go-version-file:`)
#   5. the `go` directive of go.mod (the repo default)
#
# A step is identified by its POSITION (the line its `- ` entry starts on),
# never by its `name:`. Most steps in real workflows are unnamed, so a name key
# makes every unnamed step of a job share one scope: a `GOTOOLCHAIN` pinned on
# an unrelated unnamed step then "satisfies" the pinned tool and the guard
# reports a false pass (found in review of gate#1145).
#
# Fail-closed: a requirement that cannot be resolved, a floating pin
# (`@latest`, a commit SHA), `GOTOOLCHAIN=auto` on a pinned-tool step, or a
# toolchain spec that is not a concrete version all fail the check instead of
# passing unverified.
#
# Fix a failure by either pinning the tool step to a toolchain it supports
# (`env: GOTOOLCHAIN: goX.Y.Z`) or pinning the tool to a version whose `go`
# directive the toolchain already satisfies. Do not delete the pin.
#
# Usage:  check-pinned-go-tools.sh [workflow-dir]   (default .github/workflows)
#         check-pinned-go-tools.sh --self-test      parser regression suite
set -euo pipefail

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SELF="$SELF_DIR/$(basename "${BASH_SOURCE[0]}")"

# ---------------------------------------------------------------------------
# version helpers
# ---------------------------------------------------------------------------

# 1.27 -> 1.27.0 so `sort -V` compares equal-length triples
normalize_version() {
  local v="${1#go}"
  v="${v%%[[:space:]]*}"
  while [ "${v#*.*.*}" = "$v" ]; do v="$v.0"; done
  printf '%s' "$v"
}

version_gte() { [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -n1)" = "$1" ]; }

# ---------------------------------------------------------------------------
# go directive / module proxy lookups
# ---------------------------------------------------------------------------

go_directive() { # $1 = path to a go.mod-like file
  local line
  [ -f "$1" ] || return 1
  while IFS= read -r line || [ -n "$line" ]; do
    if [[ $line =~ ^go[[:space:]]+([0-9]+(\.[0-9]+)*) ]]; then
      printf '%s' "${BASH_REMATCH[1]}"
      return 0
    fi
  done < "$1"
  return 1
}

module_go_directive_for() { # $1 = pkg@version -> prints the required go version
  local spec="$1" candidate="${1%@*}" version="${1##*@}" out gomod line
  if [ -n "${CHECK_PINNED_TOOLS_REQUIREMENTS:-}" ]; then
    # Self-test mode: hermetic requirement table ("spec=goversion;spec=goversion").
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in
        "$spec="*) printf '%s' "${line#*=}"; return 0 ;;
      esac
    done < <(printf '%s\n' "$CHECK_PINNED_TOOLS_REQUIREMENTS" | tr ';' '\n')
    return 1
  fi
  # The module path is unknown for a package path
  # (`.../golangci-lint/cmd/golangci-lint`), so strip path elements until a
  # module resolves; the first prefix that exists at that version is the one
  # Go would build from.
  while :; do
    out="$(go mod download -json "${candidate}@${version}" 2>/dev/null || true)"
    gomod="$(printf '%s' "$out" | sed -n 's/.*"GoMod": "\([^"]*\)".*/\1/p')"
    if [ -n "$gomod" ] && [ -f "$gomod" ]; then
      go_directive "$gomod"
      return $?
    fi
    case "$candidate" in
      */*) candidate="${candidate%/*}" ;;
      *) return 1 ;;
    esac
  done
}

# ---------------------------------------------------------------------------
# workflow scanning
# ---------------------------------------------------------------------------

# A workflow file is scanned twice: pass 1 collects the toolchain declarations
# of each job, pass 2 collects the pins of each step. Two passes avoid
# ordering assumptions (e.g. a `setup-go` step that appears after the pinned
# step).
#
# Globals used per file: WF_GTC, JOB_GTC, JOB_VER, JOB_VERFILE (assoc arrays);
# per whole run: CHECKED, FAILURES.

reset_file_state() {
  WF_GTC=""
  # declare -gA: a plain `X=()` would create an INDEXED array, and a non-numeric
  # subscript (the job name) would then silently address index 0.
  declare -gA JOB_GTC=()
  declare -gA JOB_VER=()
  declare -gA JOB_VERFILE=()
}

scan_toolchain_declarations() { # $1 = file
  local file="$1" line lead indent rest cur_job="" step_indent=-1 in_step=0 in_jobs=0 setup_go=0 rest_stripped key
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%$'\r'}"
    [[ -z "${line//[[:space:]]/}" ]] && continue
    [[ $line =~ ^[[:space:]]*# ]] && continue
    lead="${line%%[![:space:]]*}"
    indent=${#lead}
    rest="${line#"$lead"}"

    # Workflow-level env GOTOOLCHAIN sits before `jobs:` (indent 2).
    if [ "$indent" -eq 2 ] && [[ $rest =~ ^GOTOOLCHAIN[[:space:]]*:[[:space:]]*(.*)$ ]]; then
      [ "$in_jobs" = 0 ] && WF_GTC="${BASH_REMATCH[1]}"
      continue
    fi
    if [[ $rest == "jobs:" ]]; then
      in_jobs=1
      continue
    fi
    [ "$in_jobs" = 1 ] || { continue; }

    if [ "$indent" -eq 2 ] && [[ $rest =~ ^([A-Za-z0-9_.-]+): ]]; then
      cur_job="${BASH_REMATCH[1]}"
      in_step=0
      step_indent=-1
      setup_go=0
      continue
    fi

    if [[ $rest =~ ^-[[:space:]] ]]; then
      local dash_indent=$indent
      rest_stripped="${rest#-}"
      rest_stripped="${rest_stripped#"${rest_stripped%%[![:space:]]*}"}"
      if [ "$in_step" = 0 ] || [ "$dash_indent" -le "$step_indent" ]; then
        in_step=1
        step_indent=$dash_indent
        setup_go=0
      fi
      [[ $rest_stripped == uses:*setup-go* ]] && setup_go=1
      continue
    fi

    if [ "$in_step" = 1 ] && [ "$indent" -le "$step_indent" ]; then
      in_step=0
      setup_go=0
    fi

    if [ "$in_step" = 0 ]; then
      # Job-scoped declaration (job env, or a job-level `with:` of a reusable
      # workflow call — both apply to every step of the job).
      if [ -n "$cur_job" ] && [[ $rest =~ ^GOTOOLCHAIN[[:space:]]*:[[:space:]]*(.*)$ ]]; then
        JOB_GTC["$cur_job"]="${BASH_REMATCH[1]}"
      fi
      continue
    fi

    if [[ $rest =~ ^uses:[[:space:]]*([^[:space:]]+) ]] && [[ ${BASH_REMATCH[1]} == *setup-go* ]]; then
      setup_go=1
      continue
    fi
    if [ "$setup_go" = 1 ] && [ -n "$cur_job" ]; then
      if [[ $rest =~ ^go-version:[[:space:]]*(.*)$ ]]; then
        key="${BASH_REMATCH[1]}"
        key="${key%\"}"; key="${key#\"}"; key="${key%\'}"; key="${key#\'}"
        JOB_VER["$cur_job"]="$key"
      elif [[ $rest =~ ^go-version-file:[[:space:]]*(.*)$ ]]; then
        key="${BASH_REMATCH[1]}"
        key="${key%\"}"; key="${key#\"}"; key="${key%\'}"; key="${key#\'}"
        JOB_VERFILE["$cur_job"]="$key"
      fi
    fi
  done < "$file"
}

scan_pins() { # $1 = file ; emits file|job|step-line|step-name|pin records (one per pin)
  local file="$1" line lead indent rest cur_job="" step_indent=-1 step_line=-1 in_step=0 rest_stripped spec
  local lineno=0 step_name="" in_pins=0
  while IFS= read -r line || [ -n "$line" ]; do
    lineno=$((lineno + 1))
    line="${line%$'\r'}"
    [[ -z "${line//[[:space:]]/}" ]] && continue
    [[ $line =~ ^[[:space:]]*# ]] && continue
    lead="${line%%[![:space:]]*}"
    indent=${#lead}
    rest="${line#"$lead"}"

    if [[ $rest == "jobs:" ]]; then continue; fi

    if [ "$indent" -eq 2 ] && [[ $rest =~ ^([A-Za-z0-9_.-]+): ]]; then
      cur_job="${BASH_REMATCH[1]}"
      in_step=0
      continue
    fi

    if [[ $rest =~ ^-[[:space:]] ]]; then
      local dash_indent=$indent
      rest_stripped="${rest#-}"
      rest_stripped="${rest_stripped#"${rest_stripped%%[![:space:]]*}"}"
      if [ "$in_step" = 0 ] || [ "$dash_indent" -le "$step_indent" ]; then
        in_step=1
        step_indent=$dash_indent
        step_line=$lineno
        step_name=""
      fi
      [[ $rest_stripped =~ ^name:[[:space:]]*(.*)$ ]] && step_name="${BASH_REMATCH[1]}"
      rest="$rest_stripped"
      in_pins=1
    else
      if [ "$in_step" = 1 ] && [ "$indent" -le "$step_indent" ]; then
        in_step=0
        in_pins=0
      fi
      if [ "$in_step" = 1 ]; then
        if [ -z "$step_name" ] && [[ $rest =~ ^name:[[:space:]]*(.*)$ ]]; then
          step_name="${BASH_REMATCH[1]}"
        fi
        in_pins=1
      else
        in_pins=0
      fi
    fi

    [ "$in_pins" = 1 ] || continue
    [ -n "$cur_job" ] || continue

    while [[ $rest =~ (^|[^[:alnum:]_])go[[:space:]]+(run|install)(([[:space:]]+-{1,2}[^[:space:]]+)*)[[:space:]]+([^[:space:]]+@[^[:space:]]+) ]]; do
      spec="${BASH_REMATCH[5]}"
      spec="${spec%\"}"; spec="${spec#\"}"; spec="${spec%\'}"; spec="${spec#\'}"
      printf '%s|%s|%s|%s|%s\n' "$file" "$cur_job" "$step_line" "$step_name" "$spec"
      rest="${rest#*"${BASH_REMATCH[0]}"}"
    done
    # Fail-closed on a step that shells out to `go run`/`go install` without a
    # version pin at all (a floating ref cannot be resolved).
    if [[ $rest =~ (^|[^[:alnum:]_])go[[:space:]]+(run|install)([[:space:]]+-{1,2}[^[:space:]]+)*[[:space:]]+([^[:space:]-][^[:space:]]*) ]]; then
      spec="${BASH_REMATCH[4]}"
      case "$spec" in
        *@*) ;;
        ./*|../*|/*) continue ;;
        *) printf '%s|%s|%s|%s|%s\n' "$file" "$cur_job" "$step_line" "$step_name" "$spec" ;;
      esac
    fi
  done < "$file"
}

step_override_for() { # $1 = job, $2 = step start line, $3 = file -> GOTOOLCHAIN for that step (env key or inline)
  local job="$1" want_line="$2" file="$3" line lead indent rest cur_job="" in_step=0 step_indent=-1 step_line=-1 found="" lineno=0
  while IFS= read -r line || [ -n "$line" ]; do
    lineno=$((lineno + 1))
    line="${line%$'\r'}"
    [[ -z "${line//[[:space:]]/}" ]] && continue
    [[ $line =~ ^[[:space:]]*# ]] && continue
    lead="${line%%[![:space:]]*}"
    indent=${#lead}
    rest="${line#"$lead"}"
    if [ "$indent" -eq 2 ] && [[ $rest =~ ^([A-Za-z0-9_.-]+): ]]; then cur_job="${BASH_REMATCH[1]}"; in_step=0; continue; fi
    if [[ $rest =~ ^-[[:space:]] ]]; then
      local dash_indent=$indent
      rest="${rest#-}"
      rest="${rest#"${rest%%[![:space:]]*}"}"
      if [ "$in_step" = 0 ] || [ "$dash_indent" -le "$step_indent" ]; then
        in_step=1
        step_indent=$dash_indent
        step_line=$lineno
      fi
    else
      if [ "$in_step" = 1 ] && [ "$indent" -le "$step_indent" ]; then in_step=0; fi
    fi
    [ "$in_step" = 1 ] || continue
    [ "$cur_job" = "$job" ] || continue
    [ "$step_line" = "$want_line" ] || continue
    if [[ $rest =~ GOTOOLCHAIN[[:space:]]*=[[:space:]]*(go[0-9][0-9.]*|auto|local) ]]; then found="${BASH_REMATCH[1]}"; fi
    if [[ $rest =~ GOTOOLCHAIN[[:space:]]*:[[:space:]]*(.*)$ ]]; then found="${BASH_REMATCH[1]}"; fi
  done < "$file"
  printf '%s' "$found"
}

resolve_toolchain() { # $1 = job, $2 = step start line, $3 = file ; prints "version<TAB>source" or "!error"
  local job="$1" step_line="$2" file="$3" v="" src="" f="" spec
  v="$(step_override_for "$job" "$step_line" "$file")"
  if [ -n "$v" ]; then printf '%s\t%s\n' "$v" "step GOTOOLCHAIN=$v"; return 0; fi
  if [ -n "${JOB_GTC[$job]:-}" ]; then
    printf '%s\t%s\n' "${JOB_GTC[$job]}" "job env GOTOOLCHAIN=${JOB_GTC[$job]}"
    return 0
  fi
  if [ -n "$WF_GTC" ]; then printf '%s\t%s\n' "$WF_GTC" "workflow env GOTOOLCHAIN=$WF_GTC"; return 0; fi
  if [ -n "${JOB_VER[$job]:-}" ]; then
    printf '%s\t%s\n' "${JOB_VER[$job]}" "job setup-go go-version=${JOB_VER[$job]}"
    return 0
  fi
  f="${JOB_VERFILE[$job]:-}"
  if [ -n "$f" ]; then
    spec="$(go_directive "$f" || true)"
    if [ -n "$spec" ]; then printf '%s\t%s\n' "$spec" "job setup-go go-version-file=$f (go $spec)"; return 0; fi
    printf '!\t%s\n' "job setup-go go-version-file=$f does not exist or has no go directive"
    return 0
  fi
  printf '%s\t%s\n' "$BASE_GO" "go.mod (go $BASE_GO)"
}

check_file() { # $1 = file ; increments CHECKED / FAILURES
  local file="$1"
  reset_file_state
  scan_toolchain_declarations "$file"

  local job
  while IFS='|' read -r _f job _stepline _step spec; do
    [ -n "${spec:-}" ] || continue
    CHECKED=$((CHECKED + 1))
    # Where to point the reader: the step's name when it has one, else its
    # position, so two unnamed steps in one job stay distinguishable.
    local where
    where="${_step:-unnamed step at $file line $_stepline}"
    local required="" resolved effective source normalized_effective normalized_required
    if [[ $spec != *@* ]]; then
      printf 'FAIL %s: %s runs `go %s` with no version pin; pin a tagged version so it can be verified\n' \
        "$file" "$job" "$spec" >&2
      FAILURES=$((FAILURES + 1))
      continue
    fi
    if [[ ! $spec =~ @v[0-9] ]]; then
      printf 'FAIL %s: %s: %s is a floating ref; pin a tagged version (vX.Y.Z) so its Go requirement is verifiable\n' \
        "$file" "$job" "$spec" >&2
      FAILURES=$((FAILURES + 1))
      continue
    fi
    required="$(module_go_directive_for "$spec" || true)"
    if [ -z "$required" ]; then
      printf 'FAIL %s: %s: could not resolve %s (is the version tagged and reachable?)\n' "$file" "$job" "$spec" >&2
      FAILURES=$((FAILURES + 1))
      continue
    fi
    IFS=$'\t' read -r effective source < <(resolve_toolchain "$job" "$_stepline" "$file")
    if [[ $effective == "!"* ]]; then
      printf 'FAIL %s: %s: %s\n' "$file" "$job" "$source" >&2
      FAILURES=$((FAILURES + 1))
      continue
    fi
    case "$effective" in
      auto)
        printf 'FAIL %s: %s: step %s sets GOTOOLCHAIN=auto; pin an exact toolchain (e.g. GOTOOLCHAIN: go1.27.1) so this check can verify %s\n' \
          "$file" "$job" "$where" "$spec" >&2
        FAILURES=$((FAILURES + 1))
        continue
        ;;
      local)
        source="GOTOOLCHAIN=local with $source"
        effective="$BASE_GO"
        ;;
      go[0-9]*)
        effective="${effective#go}"
        ;;
    esac
    if [[ ! $effective =~ ^[0-9]+\.[0-9]+(\.[0-9x]+)?$ ]]; then
      printf 'FAIL %s: %s: %s declares a toolchain that is not a concrete version; pin an exact version so this check can verify %s\n' \
        "$file" "$job" "$source" "$spec" >&2
      FAILURES=$((FAILURES + 1))
      continue
    fi
    normalized_effective="$(normalize_version "${effective%.x}")"
    normalized_required="$(normalize_version "$required")"
    if version_gte "$normalized_effective" "$normalized_required"; then
      printf 'ok   %s: %s needs go >= %s, %s provides go %s\n' \
        "$file" "$spec" "$normalized_required" "$source" "$normalized_effective"
    else
      printf 'FAIL %s: %s: %s requires go >= %s but %s provides go %s\n' \
        "$file" "$where" "$spec" "$normalized_required" "$source" "$normalized_effective" >&2
      printf '     -> pin the step env to a supported toolchain (GOTOOLCHAIN: go%s+) or pin the tool to a version that supports go %s\n' \
        "$normalized_required" "$normalized_effective" >&2
      FAILURES=$((FAILURES + 1))
    fi
  done < <(scan_pins "$file")
}

check_all() { # $1 = workflow dir
  local dir="$1" f
  local files=()
  shopt -s nullglob
  files=("$dir"/*.yml "$dir"/*.yaml)
  shopt -u nullglob
  if [ "${#files[@]}" -eq 0 ]; then
    echo "check-pinned-go-tools: no workflow files (*.yml, *.yaml) in $dir" >&2
    return 1
  fi

  BASE_GO="$(go_directive go.mod || true)"
  if [ -z "$BASE_GO" ]; then
    echo "check-pinned-go-tools: no 'go' directive found in ./go.mod (run this from the repository root)" >&2
    return 1
  fi
  BASE_GO="$(normalize_version "$BASE_GO")"
  printf 'base toolchain: go %s (go.mod)\n' "$BASE_GO"

  CHECKED=0
  FAILURES=0
  for f in "${files[@]}"; do
    check_file "$f"
  done

  if [ "$CHECKED" -eq 0 ]; then
    echo "check-pinned-go-tools: no pinned Go tools found in $dir" >&2
    return 1
  fi
  if [ "$FAILURES" -gt 0 ]; then
    printf 'check-pinned-go-tools: %d of %d pinned tool(s) are not runnable with their step toolchain\n' \
      "$FAILURES" "$CHECKED" >&2
    return 1
  fi
  printf 'check-pinned-go-tools: %d pinned tool(s) verified\n' "$CHECKED"
}

# ---------------------------------------------------------------------------
# parser regression suite (hermetic: requirements come from
# CHECK_PINNED_TOOLS_REQUIREMENTS, so no network and no module cache is used)
# ---------------------------------------------------------------------------

self_test() {
  local tmp cases=0 bad=0
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN

  write_case() { # $1 = case name
    mkdir -p "$tmp/$1/workflows"
    printf 'module fixture\n\ngo 1.26.0\n' > "$tmp/$1/go.mod"
  }

  expect_case() { # $1 = case, $2 = want exit, $3 = requirements, rest = needles ("!" prefix = must be absent)
    local name="$1" want="$2" reqs="$3" out code needle ok=1
    shift 3
    cases=$((cases + 1))
    out="$(cd "$tmp/$name" && CHECK_PINNED_TOOLS_REQUIREMENTS="$reqs" bash "$SELF" workflows 2>&1)" || code=$?
    code="${code:-0}"
    if [ "$code" != "$want" ]; then
      printf '  exit %s, want %s\n' "$code" "$want"
      ok=0
    fi
    for needle in "$@"; do
      case "$needle" in
        "!"*)
          if [[ $out == *"${needle#!}"* ]]; then printf '  unexpected output: %s\n' "${needle#!}"; ok=0; fi
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

  # 1. step-level override satisfies the tool
  write_case step-toolchain-ok
  cat > "$tmp/step-toolchain-ok/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: pinned tool
        env:
          GOTOOLCHAIN: go1.28.1
        run: go run example.com/tool@v1.0.0 ./...
YAML
  expect_case step-toolchain-ok 0 "example.com/tool@v1.0.0=1.28.0" "ok" "step GOTOOLCHAIN=go1.28.1"

  # 2. no override -> the tool outgrew go.mod
  write_case missing-toolchain
  cat > "$tmp/missing-toolchain/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: pinned tool
        run: go run example.com/tool@v1.0.0 ./...
YAML
  expect_case missing-toolchain 1 "example.com/tool@v1.0.0=1.28.0" \
    "FAIL" "requires go >= 1.28.0" "go.mod (go 1.26.0) provides go 1.26.0" "1 of 1"

  # 3. regression: a pin in the LAST step of the alphabetically-last file must
  #    still be checked (the parser must not rely on an end-of-file flush)
  write_case last-step-last-file
  cat > "$tmp/last-step-last-file/workflows/a-first.yml" <<'YAML'
name: ci
on: push
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - name: fine tool
        env:
          GOTOOLCHAIN: go1.28.1
        run: go run example.com/ok@v1.0.0 ./...
YAML
  cat > "$tmp/last-step-last-file/workflows/z-last.yml" <<'YAML'
name: ci
on: push
jobs:
  z:
    runs-on: ubuntu-latest
    steps:
      - name: first step
        run: echo hello
      - name: last step pin
        run: go run example.com/last@v1.0.0 ./...
YAML
  expect_case last-step-last-file 1 "example.com/ok@v1.0.0=1.27.0;example.com/last@v1.0.0=1.28.0" \
    "FAIL" "example.com/last@v1.0.0" "1 of 2"

  # 4. job-level env GOTOOLCHAIN satisfies the tool
  write_case job-level-toolchain-ok
  cat > "$tmp/job-level-toolchain-ok/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    env:
      GOTOOLCHAIN: go1.28.1
    steps:
      - name: pinned tool
        run: go run example.com/tool@v1.0.0 ./...
YAML
  expect_case job-level-toolchain-ok 0 "example.com/tool@v1.0.0=1.28.0" "job env GOTOOLCHAIN=go1.28.1"

  # 5. regression: a job-level toolchain that is too old must fail (it used to
  #    be invisible, so the check passed while reality failed)
  write_case job-level-toolchain-too-low
  cat > "$tmp/job-level-toolchain-too-low/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    env:
      GOTOOLCHAIN: go1.22.0
    steps:
      - name: pinned tool
        run: go run example.com/tool@v1.0.0 ./...
YAML
  expect_case job-level-toolchain-too-low 1 "example.com/tool@v1.0.0=1.23.0" \
    "FAIL" "job env GOTOOLCHAIN=go1.22.0 provides go 1.22.0"

  # 6. workflow-level env GOTOOLCHAIN satisfies the tool
  write_case workflow-level-toolchain
  cat > "$tmp/workflow-level-toolchain/workflows/ci.yml" <<'YAML'
name: ci
on: push
env:
  GOTOOLCHAIN: go1.28.1
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: pinned tool
        run: go run example.com/tool@v1.0.0 ./...
YAML
  expect_case workflow-level-toolchain 0 "example.com/tool@v1.0.0=1.28.0" "workflow env GOTOOLCHAIN=go1.28.1"

  # 7. GOTOOLCHAIN=auto is not an acceptable pin on a pinned-tool step
  write_case step-toolchain-auto
  cat > "$tmp/step-toolchain-auto/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: pinned tool
        env:
          GOTOOLCHAIN: auto
        run: go run example.com/tool@v1.0.0 ./...
YAML
  expect_case step-toolchain-auto 1 "example.com/tool@v1.0.0=1.28.0" "GOTOOLCHAIN=auto"

  # 8. floating refs cannot be verified
  write_case floating-ref
  cat > "$tmp/floating-ref/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: pinned tool
        run: go run example.com/tool@latest ./...
YAML
  expect_case floating-ref 1 "example.com/tool@latest=1.28.0" "FAIL" "@latest" "floating ref"

  # 9. regression: a flag between the subcommand and the spec must not hide the pin
  write_case flags-before-spec
  cat > "$tmp/flags-before-spec/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: pinned tool
        run: go run -mod=mod example.com/tool@v1.0.0 ./...
YAML
  expect_case flags-before-spec 1 "example.com/tool@v1.0.0=1.28.0" "FAIL" "example.com/tool@v1.0.0"

  # 10. comments are not pins
  write_case comment-not-a-pin
  cat > "$tmp/comment-not-a-pin/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      # prose: never `go run example.com/ghost@v9.9.9` here
      - name: fine tool
        env:
          GOTOOLCHAIN: go1.28.1
        run: go run example.com/ok@v1.0.0 ./...
YAML
  expect_case comment-not-a-pin 0 "example.com/ok@v1.0.0=1.28.0" "!example.com/ghost@v9.9.9"

  # 11. a setup-go version pin is the job toolchain
  write_case setupgo-go-version
  cat > "$tmp/setupgo-go-version/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: Setup Go
        uses: actions/setup-go@v6
        with:
          go-version: '1.28.x'
      - name: pinned tool
        run: go run example.com/tool@v1.0.0 ./...
YAML
  expect_case setupgo-go-version 0 "example.com/tool@v1.0.0=1.28.0" "go-version=1.28.x"

  # 12. inline GOTOOLCHAIN=... on the run line counts
  write_case inline-step-toolchain
  cat > "$tmp/inline-step-toolchain/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: pinned tool
        run: GOTOOLCHAIN=go1.28.1 go run example.com/tool@v1.0.0 ./...
YAML
  expect_case inline-step-toolchain 0 "example.com/tool@v1.0.0=1.28.0" "step GOTOOLCHAIN=go1.28.1"

  # 13. multi-line run blocks are scanned
  write_case multiline-run
  cat > "$tmp/multiline-run/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: pinned tool
        run: |
          set -euo pipefail
          go install example.com/tool@v1.0.0
          example.com/tool --version
YAML
  expect_case multiline-run 1 "example.com/tool@v1.0.0=1.28.0" "FAIL" "example.com/tool@v1.0.0"

  # 14. regression: steps are scoped by POSITION, not by `name:`. Two unnamed
  #     steps of one job must not share a scope, otherwise a GOTOOLCHAIN pinned
  #     on an unrelated unnamed step silently "satisfies" the pinned tool.
  write_case unnamed-step-scope-leak
  cat > "$tmp/unnamed-step-scope-leak/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - run: |
          go run example.com/tool@v1.0.0 ./...
      - run: go test ./...
      - env:
          GOTOOLCHAIN: go1.28.1
        run: go vet ./...
YAML
  expect_case unnamed-step-scope-leak 1 "example.com/tool@v1.0.0=1.28.0" \
    "FAIL" "requires go >= 1.28.0" "go.mod (go 1.26.0) provides go 1.26.0" "!step GOTOOLCHAIN=go1.28.1"

  # 15. control for 14: an unnamed step's OWN toolchain still counts
  write_case unnamed-step-own-toolchain
  cat > "$tmp/unnamed-step-own-toolchain/workflows/ci.yml" <<'YAML'
name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - env:
          GOTOOLCHAIN: go1.28.1
        run: go run example.com/tool@v1.0.0 ./...
      - run: go vet ./...
YAML
  expect_case unnamed-step-own-toolchain 0 "example.com/tool@v1.0.0=1.28.0" "step GOTOOLCHAIN=go1.28.1"

  printf 'check-pinned-go-tools: self-test %d/%d cases passed\n' "$((cases - bad))" "$cases"
  [ "$bad" -eq 0 ]
}

# ---------------------------------------------------------------------------
# entry point
# ---------------------------------------------------------------------------

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

check_all "${1:-.github/workflows}"
