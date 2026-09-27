package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Every release PR produces a red `ci` run and it can never go green. That is
// not a broken job, not a flaky runner and not the trigger: it is GitHub's
// documented approval gate for pull requests that a workflow created with
// `GITHUB_TOKEN`.
//
//	"When a pull request is created or updated by a workflow using
//	 `GITHUB_TOKEN`, `pull_request` events with the `opened`, `synchronize`, or
//	 `reopened` activity types create workflow runs that require approval. A
//	 user with write access to the repository can approve these runs from the
//	 pull request page. With the exception of `workflow_dispatch` and
//	 `repository_dispatch`, other `GITHUB_TOKEN`-triggered events do not create
//	 workflow runs at all."
//	— GitHub docs, "Events that trigger workflows" -> `pull_request`
//
// Observed on this repository (2026-09-27, release 0.74.24): every run that
// ever existed on `release-please--branches--master--components--gate` is this
// shape — 61 of 61 are `event=pull_request`, `ci`, actor `github-actions[bot]`
// and **0 jobs** (`conclusion=failure` for 60 of them; run 34519830891 from
// 0.73.9 still shows the raw `action_required`). There are no `push` runs on
// that branch at all: release-please pushes with `GITHUB_TOKEN`, and a
// `GITHUB_TOKEN` push creates no workflow runs. So on a release PR this
// workflow contributes *nothing* that branch protection can match: the gated
// run publishes no check-run (check suite 98365089871 has zero check runs), and
// the PR's own status rollup shows only the unrelated "Cloudflare Pages" check.
//
// The red run is instead visible in the Actions list and on the release branch
// commit, which is exactly why it looks like a defect worth "fixing".
//
// Consequence, and the reason this test exists: no context this workflow
// reports may ever become a required status check of `master` — neither the
// workflow-level `ci` / `ci / <job>` entry nor the individual job contexts
// (`lint`, `pinned-tools`, `test (…/…)`, `docker-smoke (…)`). A release PR can
// never satisfy them, because there is no executed run behind them on the
// release branch: requiring one leaves every release PR permanently
// unsatisfied, the auto-merge step in release-please.yml (`gh pr merge
// --merge`, deliberately no `--admin`) fails, `trigger-release` and
// `dispatch-moxy-bump` are skipped and the release silently never publishes —
// the same stall shape as the v0.74.21 payload bug. master today carries no
// required status checks at all (branch protection reads 404), so the trap is
// latent, not live.
//
// Do not "fix" the phantom by touching `paths:`/`branches:` filters: the
// approval requirement comes from the `GITHUB_TOKEN` activity, not from the
// trigger, and a non-matching filter produces no run at all rather than a green
// one (`web.yml` is this repository's control for that, asserted below) — while
// the gated run publishes no check-run to remove either. The only real fix is
// to have release-please open its PR with a GitHub App/PAT token instead of
// `GITHUB_TOKEN` (GitHub docs, "Triggering a workflow from a workflow": an App
// installation token or PAT "also lets `pull_request` workflows run
// automatically (without the approval prompt described above) when the pull
// request is created or updated by automation"). That is a credential change
// needing Robin's approval, so it is deliberately not done here; the accepted
// noise is documented at the `on:` block of ci.yml and pinned by this test.
const releaseCheckContractPrefix = "release-check contract:"

const (
	releaseCheckCIWorkflowPath      = ciWorkflowPath
	releaseCheckControlWorkflowPath = webWorkflowPath

	// releaseCheckCIWorkflowName is the workflow name GitHub reports for the
	// gated release-PR run and the name the acceptance note in ci.yml uses.
	releaseCheckCIWorkflowName = "ci"
)

// releaseCheckReleasePRFiles is the file set release-please writes into its own
// release PR. Observed on #1208 (0.74.24) and unchanged across the releases
// listed above; it is the input the approval-gate analysis depends on.
var releaseCheckReleasePRFiles = []string{
	".release-please-manifest.json",
	"CHANGELOG.md",
}

// releaseCheckNoteAnchors are the load-bearing phrases of the acceptance note in
// ci.yml. The note is the only place a future reader learns *why* the red run is
// accepted and *why* no context from this workflow may be required, so losing
// any of them has to fail the build rather than silently un-document the trap.
var releaseCheckNoteAnchors = []string{
	"GitHub's documented approval gate",
	"GITHUB_TOKEN",
	"zero jobs",
	"must never be added to master's required status checks",
	"release_check_contract_test.go",
}

// releaseCheckContract is every workflow fact the acceptance decision rests on.
// It is a plain value so the mutation table below can rebuild it.
type releaseCheckContract struct {
	ciWorkflowName     string
	ciPullRequestTypes []string
	// ciPullRequestPaths is the `paths` filter of ci.yml's pull_request
	// trigger; ciHasPathsFilter distinguishes "no filter" (today's shape, every
	// PR triggers the run) from "filter that happens to match".
	ciPullRequestPaths []string
	ciHasPathsFilter   bool
	// controlPullRequestPaths is web.yml's pull_request filter. It must keep
	// matching no release PR file: that is the control proving a non-matching
	// filter yields no run at all.
	controlPullRequestPaths []string
	// releasePleaseTokenInput is the `token:` the release-please action step
	// receives. Empty means it uses `GITHUB_TOKEN`, i.e. the approval gate
	// applies.
	releasePleaseTokenInput string
	// releasePleaseCommands is every shell command release-please.yml executes
	// (comments dropped, continuations joined).
	releasePleaseCommands []string
}

// releaseCheckLiveContract reads the workflows and assembles the contract.
func releaseCheckLiveContract(t *testing.T) releaseCheckContract {
	t.Helper()
	ci := releaseCheckReadWorkflow(t, releaseCheckCIWorkflowPath)
	control := releaseCheckReadWorkflow(t, releaseCheckControlWorkflowPath)
	types, paths, hasPaths := releaseCheckPullRequestTrigger(t, releaseCheckCIWorkflowPath, ci)
	return releaseCheckContract{
		ciWorkflowName:          releaseCheckScalar(ci, "name"),
		ciPullRequestTypes:      types,
		ciPullRequestPaths:      paths,
		ciHasPathsFilter:        hasPaths,
		controlPullRequestPaths: releaseCheckTriggerPaths(t, releaseCheckControlWorkflowPath, control, "pull_request"),
		releasePleaseTokenInput: releaseCheckReleasePleaseToken(t, "."),
		releasePleaseCommands:   releaseCheckReleasePleaseCommands(t, "."),
	}
}

// validateReleaseCheckContract fails when any fact the accepted-noise decision
// rests on has changed. The messages name the disposition so a future reader
// re-decides it instead of deleting the guard.
func validateReleaseCheckContract(c releaseCheckContract, releaseFiles []string) error {
	if c.ciWorkflowName != releaseCheckCIWorkflowName {
		return fmt.Errorf("the workflow that reds every release PR is no longer named %q (got %q); the acceptance "+
			"note in %s names it explicitly, so re-check the mechanism before renaming it",
			releaseCheckCIWorkflowName, c.ciWorkflowName, releaseCheckCIWorkflowPath)
	}

	if !releaseCheckContains(c.ciPullRequestTypes, "opened") {
		return fmt.Errorf("%s no longer triggers on the `opened` activity type (types %v): that is the activity "+
			"type a `GITHUB_TOKEN`-created pull request hits, so without it the approval-gated run does not exist "+
			"and the disposition in %s has to be re-decided (the release PR would then report nothing at all, "+
			"not something green)", releaseCheckCIWorkflowPath, c.ciPullRequestTypes, releaseCheckCIWorkflowPath)
	}

	if c.ciHasPathsFilter {
		var matched []string
		for _, file := range releaseFiles {
			if releaseCheckMatchesAny(c.ciPullRequestPaths, file) {
				matched = append(matched, file)
			}
		}
		if len(matched) == 0 {
			return fmt.Errorf("%s no longer triggers on a release PR: none of %v matches the release PR file set %v. "+
				"If this was meant to silence the red run, note that a filter can only turn it into *no run at all* "+
				"(see the %s control below) — never into a green one — and that the gated run publishes no check-run "+
				"to remove. The acceptance note in %s and the never-require rule for `%s` have to be re-decided "+
				"together with this test", releaseCheckCIWorkflowPath, releaseFiles, c.ciPullRequestPaths,
				releaseCheckControlWorkflowPath, releaseCheckCIWorkflowPath, releaseCheckCIWorkflowName)
		}
	}

	for _, file := range releaseFiles {
		if releaseCheckMatchesAny(c.controlPullRequestPaths, file) {
			return fmt.Errorf("%s now triggers on %q; the control that shows a non-matching filter creates no run "+
				"at all — and therefore that the red release-PR run is the approval gate, not a filter artefact — "+
				"no longer holds", releaseCheckControlWorkflowPath, file)
		}
	}

	if c.releasePleaseTokenInput != "" {
		return fmt.Errorf("the release-please action now receives a token (%q), so its pull request is no longer "+
			"created with GITHUB_TOKEN and the approval gate described in %s does not apply anymore. Remove the "+
			"acceptance note, verify whether the release-PR run is now green, and then reconsider both the "+
			"never-require rule and this test", c.releasePleaseTokenInput, releaseCheckCIWorkflowPath)
	}

	commands := strings.Join(c.releasePleaseCommands, "\n")
	if !strings.Contains(commands, "--merge") {
		return fmt.Errorf("the release-please auto-merge step no longer merges synchronously (`--merge`); %s's red "+
			"release-PR run is only harmless while the chain never waits on checks a release PR cannot satisfy",
			releaseCheckCIWorkflowPath)
	}
	if strings.Contains(commands, "--auto") {
		return fmt.Errorf("the release-please auto-merge step now waits for checks (`--auto`); a release PR can never "+
			"satisfy a context from %s (the gated run has zero jobs and publishes no check-run), so the chain would "+
			"stall exactly like the v0.74.21 payload stall", releaseCheckCIWorkflowPath)
	}
	if strings.Contains(commands, "--admin") {
		return fmt.Errorf("the release-please auto-merge step now bypasses branch protection (`--admin`), which would "+
			"hide a genuinely red check on a release PR and contradicts the never-require rule documented in %s",
			releaseCheckCIWorkflowPath)
	}

	for _, fragment := range []string{"check-runs", "/statuses", "-f name=", "-f context="} {
		if strings.Contains(commands, fragment) {
			return fmt.Errorf("release-please.yml now publishes a check-run or commit status (%q): fabricating a "+
				"context on a release PR could satisfy a required `%s` with a synthetic green. The release branch "+
				"must only ever carry the approval-gated run that %s documents",
				fragment, releaseCheckCIWorkflowName, releaseCheckCIWorkflowPath)
		}
	}
	return nil
}

// TestReleaseCheckContractMatchesLiveWorkflows is the guard: the live workflows
// must still satisfy every fact the acceptance note in ci.yml depends on.
func TestReleaseCheckContractMatchesLiveWorkflows(t *testing.T) {
	contract := releaseCheckLiveContract(t)
	if err := validateReleaseCheckContract(contract, releaseCheckReleasePRFiles); err != nil {
		t.Fatalf("%s %v", releaseCheckContractPrefix, err)
	}
}

// TestReleaseCheckAcceptanceNoteIsPresentAndComplete pins the documentation
// half of the disposition. Without the note the red release-PR run looks like a
// defect (and like a free required check), which is how the trap gets armed.
func TestReleaseCheckAcceptanceNoteIsPresentAndComplete(t *testing.T) {
	if err := validateReleaseCheckNote(readRepoFile(t, releaseCheckCIWorkflowPath)); err != nil {
		t.Fatalf("%s %v", releaseCheckContractPrefix, err)
	}
}

// validateReleaseCheckNote fails when the acceptance note in ci.yml no longer
// names the mechanism, the observed shape, the rule or the guard. It matches
// against the note's prose (comment markers stripped, lines joined), so
// rewrapping the note is free while losing a phrase is not.
func validateReleaseCheckNote(text string) error {
	prose := releaseCheckProse(text)
	for _, anchor := range releaseCheckNoteAnchors {
		if !strings.Contains(prose, anchor) {
			return fmt.Errorf("%s no longer documents %q; the acceptance note must name the mechanism (GitHub's "+
				"approval gate for `GITHUB_TOKEN` pull requests), the observed shape (zero jobs), the rule (no "+
				"context from this workflow may be required on master) and point at this test",
				releaseCheckCIWorkflowPath, anchor)
		}
	}
	return nil
}

// releaseCheckProse flattens a YAML file into the prose a reader sees: comment
// markers dropped, each line trimmed, lines joined by one space.
func releaseCheckProse(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return " " + strings.Join(lines, " ") + " "
}

// TestReleaseCheckNoteGuardRejectsMutations proves the note guard has teeth:
// dropping any single load-bearing phrase must be rejected.
func TestReleaseCheckNoteGuardRejectsMutations(t *testing.T) {
	note := readRepoFile(t, releaseCheckCIWorkflowPath)
	if err := validateReleaseCheckNote(note); err != nil {
		t.Fatalf("%s %v", releaseCheckContractPrefix, err)
	}
	for _, anchor := range releaseCheckNoteAnchors {
		t.Run(strings.ReplaceAll(anchor, " ", "-"), func(t *testing.T) {
			if err := validateReleaseCheckNote(strings.ReplaceAll(note, anchor, "")); err == nil {
				t.Fatalf("%s removing %q from the acceptance note was accepted", releaseCheckContractPrefix, anchor)
			}
		})
	}
}

// TestReleaseCheckContractRejectsMutations proves the guard has teeth: each
// mutation below invalidates the mechanism the disposition rests on and must be
// rejected with a contract-prefixed error.
func TestReleaseCheckContractRejectsMutations(t *testing.T) {
	live := releaseCheckLiveContract(t)
	mutations := map[string]func(c releaseCheckContract) releaseCheckContract{
		"ci-renamed": func(c releaseCheckContract) releaseCheckContract {
			c.ciWorkflowName = "ci-gate"
			return c
		},
		"opened-activity-type-dropped": func(c releaseCheckContract) releaseCheckContract {
			c.ciPullRequestTypes = []string{"reopened"}
			return c
		},
		"ci-paths-filter-added-that-excludes-release-files": func(c releaseCheckContract) releaseCheckContract {
			c.ciHasPathsFilter = true
			c.ciPullRequestPaths = []string{"dotweb/**", "examples/**"}
			return c
		},
		"control-workflow-now-matches-release-files": func(c releaseCheckContract) releaseCheckContract {
			c.controlPullRequestPaths = append([]string(nil), c.controlPullRequestPaths...)
			c.controlPullRequestPaths = append(c.controlPullRequestPaths, "CHANGELOG.md")
			return c
		},
		"release-please-token-added": func(c releaseCheckContract) releaseCheckContract {
			c.releasePleaseTokenInput = "${{ secrets.RELEASE_PLEASE_TOKEN }}"
			return c
		},
		"automerge-switched-to-auto": func(c releaseCheckContract) releaseCheckContract {
			c.releasePleaseCommands = append(append([]string(nil), c.releasePleaseCommands...), "gh pr merge \"$PR_NUMBER\" --merge --auto")
			return c
		},
		"automerge-switched-to-admin": func(c releaseCheckContract) releaseCheckContract {
			c.releasePleaseCommands = append(append([]string(nil), c.releasePleaseCommands...), "gh pr merge \"$PR_NUMBER\" --merge --admin")
			return c
		},
		"automerge-no-longer-sync": func(c releaseCheckContract) releaseCheckContract {
			c.releasePleaseCommands = []string{"gh pr merge \"$PR_NUMBER\""}
			return c
		},
		"ci-context-fabricated": func(c releaseCheckContract) releaseCheckContract {
			c.releasePleaseCommands = append(append([]string(nil), c.releasePleaseCommands...),
				"gh api --method POST \"/repos/$GITHUB_REPOSITORY/check-runs\" -f name='ci' -f conclusion=success")
			return c
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			err := validateReleaseCheckContract(mutate(live), releaseCheckReleasePRFiles)
			if err == nil {
				t.Fatalf("%s mutation %q was accepted", releaseCheckContractPrefix, name)
			}
			if !strings.Contains(err.Error(), releaseCheckCIWorkflowPath) &&
				!strings.Contains(err.Error(), releaseCheckControlWorkflowPath) &&
				!strings.Contains(err.Error(), releasePleaseWorkflowPath) {
				t.Fatalf("%s mutation %q failed without naming the disposition: %v", releaseCheckContractPrefix, name, err)
			}
		})
	}
}

// TestReleaseCheckContractAcceptsMatchingCiPathsFilter keeps the failure above
// honest: a `paths` filter that still matches the release PR is accepted, so the
// guard rejects the *defect* (the release PR silently stops triggering) rather
// than the mere presence of a filter.
func TestReleaseCheckContractAcceptsMatchingCiPathsFilter(t *testing.T) {
	live := releaseCheckLiveContract(t)
	live.ciHasPathsFilter = true
	live.ciPullRequestPaths = []string{"go/**", "CHANGELOG.md"}
	if err := validateReleaseCheckContract(live, releaseCheckReleasePRFiles); err != nil {
		t.Fatalf("%s a ci.yml `paths` filter that still matches the release PR must be accepted, got: %v",
			releaseCheckContractPrefix, err)
	}
}

// TestReleaseCheckPathMatcher pins the matcher used above with the patterns this
// repository actually writes: `**` spans separators, `*`/`?` stay inside one
// segment.
func TestReleaseCheckPathMatcher(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{".web/**", ".web/worker/index.ts", true},
		{".web/**", "web/worker/index.ts", false},
		{".web/**", "CHANGELOG.md", false},
		{".github/workflows/web.yml", ".github/workflows/web.yml", true},
		{".github/workflows/web.yml", ".github/workflows/ci.yml", false},
		{"CHANGELOG.md", "CHANGELOG.md", true},
		{".release-please-manifest.json", ".release-please-manifest.json", true},
		{"go/**", "go/version.go", true},
		{"go/**", "gofmt/x.go", false},
		{"go/**", "go/a/b/c.go", true},
		{"rust/*", "rust/Cargo.toml", true},
		{"rust/*", "rust/src/version.rs", false},
		{"docs/?.md", "docs/a.md", true},
		{"docs/?.md", "docs/ab.md", false},
	}
	for _, tc := range cases {
		if got := releaseCheckMatchPath(tc.pattern, tc.path); got != tc.want {
			t.Errorf("%s path %q against pattern %q = %v, want %v", releaseCheckContractPrefix, tc.path, tc.pattern, got, tc.want)
		}
	}
}

func releaseCheckContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// releaseCheckMatchesAny reports whether the file matches at least one pattern.
func releaseCheckMatchesAny(patterns []string, file string) bool {
	for _, pattern := range patterns {
		if releaseCheckMatchPath(pattern, file) {
			return true
		}
	}
	return false
}

// releaseCheckMatchPath implements the subset of GitHub's `paths` globbing this
// repository uses: `**` matches across path separators, `*` and `?` match
// within a single segment.
func releaseCheckMatchPath(pattern, path string) bool {
	return releaseCheckMatchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

func releaseCheckMatchSegments(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(path); i++ {
			if releaseCheckMatchSegments(pattern[1:], path[i:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	if !releaseCheckMatchSegment(pattern[0], path[0]) {
		return false
	}
	return releaseCheckMatchSegments(pattern[1:], path[1:])
}

func releaseCheckMatchSegment(pattern, segment string) bool {
	if pattern == "" {
		return segment == ""
	}
	switch pattern[0] {
	case '*':
		// `*` never crosses a separator, so it can only consume the rest of
		// this segment.
		for i := 0; i <= len(segment); i++ {
			if releaseCheckMatchSegment(pattern[1:], segment[i:]) {
				return true
			}
		}
		return false
	case '?':
		if segment == "" {
			return false
		}
		return releaseCheckMatchSegment(pattern[1:], segment[1:])
	default:
		if segment == "" || segment[0] != pattern[0] {
			return false
		}
		return releaseCheckMatchSegment(pattern[1:], segment[1:])
	}
}

// releaseCheckReadWorkflow parses a workflow file as a YAML node tree, so the
// `on:` key is read as the literal scalar GitHub writes rather than as the
// boolean a YAML 1.1 resolver would make of it.
func releaseCheckReadWorkflow(t *testing.T, path string) *yaml.Node {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s %v", releaseCheckContractPrefix, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(contents, &doc); err != nil {
		t.Fatalf("%s %s is not parseable: %v", releaseCheckContractPrefix, path, err)
	}
	return &doc
}

func releaseCheckMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func releaseCheckScalar(doc *yaml.Node, key string) string {
	node := releaseCheckMappingValue(doc.Content[0], key)
	if node == nil {
		return ""
	}
	return node.Value
}

// releaseCheckPullRequestTrigger returns the `types` and `paths` of a workflow's
// pull_request trigger, plus whether a paths filter is declared at all.
func releaseCheckPullRequestTrigger(t *testing.T, path string, doc *yaml.Node) ([]string, []string, bool) {
	t.Helper()
	event := releaseCheckEvent(t, path, doc, "pull_request")
	if event.Kind == yaml.ScalarNode {
		// `pull_request:` with no mapping: no types, no filter.
		return nil, nil, false
	}
	var types []string
	if node := releaseCheckMappingValue(event, "types"); node != nil {
		for _, item := range node.Content {
			types = append(types, item.Value)
		}
	}
	pathsNode := releaseCheckMappingValue(event, "paths")
	if pathsNode == nil {
		return types, nil, false
	}
	var paths []string
	for _, item := range pathsNode.Content {
		paths = append(paths, item.Value)
	}
	return types, paths, true
}

// releaseCheckTriggerPaths returns the `paths` a workflow declares for an event
// trigger; a trigger without a filter has none, which callers treat as "matches
// everything".
func releaseCheckTriggerPaths(t *testing.T, path string, doc *yaml.Node, event string) []string {
	t.Helper()
	node := releaseCheckEvent(t, path, doc, event)
	if node.Kind == yaml.ScalarNode {
		return nil
	}
	pathsNode := releaseCheckMappingValue(node, "paths")
	if pathsNode == nil {
		return nil
	}
	var paths []string
	for _, item := range pathsNode.Content {
		paths = append(paths, item.Value)
	}
	return paths
}

func releaseCheckEvent(t *testing.T, path string, doc *yaml.Node, event string) *yaml.Node {
	t.Helper()
	on := releaseCheckMappingValue(doc.Content[0], "on")
	if on == nil {
		t.Fatalf("%s %s declares no `on:` block", releaseCheckContractPrefix, path)
	}
	eventNode := releaseCheckMappingValue(on, event)
	if eventNode == nil {
		t.Fatalf("%s %s does not trigger on %s", releaseCheckContractPrefix, path, event)
	}
	return eventNode
}

// releaseCheckReleasePleaseToken returns the `token:` the release-please action
// step receives (empty means it uses the runner's GITHUB_TOKEN).
func releaseCheckReleasePleaseToken(t *testing.T, dir string) string {
	t.Helper()
	step := releaseCheckReleasePleaseActionStep(t, dir)
	with := releaseCheckMappingValue(step, "with")
	if with == nil {
		return ""
	}
	if token := releaseCheckMappingValue(with, "token"); token != nil {
		return token.Value
	}
	return ""
}

func releaseCheckReleasePleaseActionStep(t *testing.T, dir string) *yaml.Node {
	t.Helper()
	workflow := releaseCheckReadWorkflow(t, filepath.Join(dir, releasePleaseWorkflowPath))
	job := releaseCheckMappingValue(releaseCheckMappingValue(workflow.Content[0], "jobs"), "release-please")
	if job == nil {
		t.Fatalf("%s %s has no release-please job", releaseCheckContractPrefix, releasePleaseWorkflowPath)
	}
	for _, step := range releaseCheckMappingValue(job, "steps").Content {
		uses := releaseCheckMappingValue(step, "uses")
		if uses == nil || !strings.Contains(uses.Value, "release-please-action") {
			continue
		}
		return step
	}
	t.Fatalf("%s %s no longer uses release-please-action; the release PR (and therefore the approval-gated `%s` "+
		"run) no longer exists", releaseCheckContractPrefix, releasePleaseWorkflowPath, releaseCheckCIWorkflowName)
	return nil
}

// releaseCheckReleasePleaseCommands returns every shell command the
// release-please workflow executes, with comments dropped and line
// continuations joined — so a command merely mentioned in a comment (the step
// documents `--merge`, not `--auto`) can never satisfy or trip an assertion.
func releaseCheckReleasePleaseCommands(t *testing.T, dir string) []string {
	t.Helper()
	workflow := releaseCheckReadWorkflow(t, filepath.Join(dir, releasePleaseWorkflowPath))
	jobs := releaseCheckMappingValue(workflow.Content[0], "jobs")
	if jobs == nil {
		t.Fatalf("%s %s has no jobs", releaseCheckContractPrefix, releasePleaseWorkflowPath)
	}
	var commands []string
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		steps := releaseCheckMappingValue(jobs.Content[i+1], "steps")
		if steps == nil {
			continue
		}
		for _, step := range steps.Content {
			run := releaseCheckMappingValue(step, "run")
			if run == nil {
				continue
			}
			commands = append(commands, releasePleaseShellCommands(run.Value)...)
		}
	}
	return commands
}
