package main

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// golangci-lint intermittently never reaches analysis on this repository's
// runners, and each occurrence used to cost either a red PR or a skipped
// release publish:
//
//   level=warning msg="[runner] Can't run linter goanalysis_metalinter:
//     buildir: failed to load package : could not load export data:
//     no export data for \"golang.org/x/image/math/f32\""
//   level=error msg="Running error: can't run linter goanalysis_metalinter ..."
//     — 2026-11-13, run 43448862215 (PR #1337) job 131965443742, exit 3
//
// The same broken package load also reports as a burst of bogus typecheck
// diagnostics against code that compiles — 51 lines of "type
// *connectedPlayer has no field or method Type (typecheck)" in 44 seconds on
// release gate run 43448561385 job 131964515714, whose sibling test jobs were
// green on the same head. That run's `trigger-release / lint` failure skipped
// release-build, image-build, verify-release-tag, publish-images,
// publish-release and dispatch-moxy-bump for tag v0.79.1: the tag existed, the
// release had zero assets, and only a re-dispatch (run 43448561447) published
// them. Nothing was wrong with the tagged tree; the retry linted it clean.
//
// So the lint step may not be a bare `golangci-lint run`: it must warm the
// build cache, start from a clean linter cache, retry exactly the loader
// class, and fail closed on everything else (a real finding is still a red
// step — after the retry). That behaviour lives in
// .github/scripts/run-golangci-lint.sh, and these tests pin the wiring and the
// classification so a future edit cannot quietly turn a finding into a retry
// (silencing the gate) or a flake into a skipped publish (the defect above).

const (
	ciWorkflowFile             = ".github/workflows/ci.yml"
	releasePublishWorkflowFile = ".github/workflows/release-publish.yml"
	releasePleaseWorkflowFile  = ".github/workflows/release-please.yml"
	lintWrapperPath            = ".github/scripts/run-golangci-lint.sh"

	lintWrapperStepName  = "golangci-lint"
	lintWrapperSelfTest  = ".github/scripts/run-golangci-lint.sh --self-test"
	lintWrapperRun       = ".github/scripts/run-golangci-lint.sh"
	lintInstallStepName  = "Install golangci-lint"
	lintSelfTestStepName = "Self-test the linter wrapper"
	verifyAssetsStep     = "Verify published release assets"
)

// lintPinRegexp matches the version pin of the linter install step.
var lintPinRegexp = regexp.MustCompile(`golangci-lint/cmd/golangci-lint@(v[0-9]+\.[0-9]+\.[0-9]+)`)

type flakeWorkflowStep struct {
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	If   string `yaml:"if"`
	Run  string `yaml:"run"`
}

type flakeWorkflow struct {
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress *bool  `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Needs any                 `yaml:"needs"`
		Steps []flakeWorkflowStep `yaml:"steps"`
	} `yaml:"jobs"`
}

func readFlakeWorkflow(t *testing.T, path string) flakeWorkflow {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var workflow flakeWorkflow
	if err := yaml.Unmarshal([]byte(normalizeWorkflowLineEndings(string(raw))), &workflow); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return workflow
}

// lintPin returns the golangci-lint version the workflow's install step pins.
func lintPin(t *testing.T, workflow flakeWorkflow) string {
	t.Helper()

	lintJob, ok := workflow.Jobs["lint"]
	if !ok {
		t.Fatalf("workflow has no `lint` job")
	}
	for _, step := range lintJob.Steps {
		if step.Name != lintInstallStepName {
			continue
		}
		matches := lintPinRegexp.FindStringSubmatch(step.Run)
		if matches == nil {
			t.Fatalf("%q step does not pin a golangci-lint version: %q", lintInstallStepName, step.Run)
		}
		return matches[1]
	}
	t.Fatalf("workflow has no %q step", lintInstallStepName)
	return ""
}

// TestLintJobsRunTheFlakeTolerantWrapper pins both lint jobs — the per-branch
// `ci` one and the release gate's — onto the wrapper. A bare
// `golangci-lint run` here is the regression: it is what failed twice on
// 2026-11-13 and what skipped the v0.79.1 publish.
func TestLintJobsRunTheFlakeTolerantWrapper(t *testing.T) {
	for _, path := range []string{ciWorkflowFile, releasePublishWorkflowFile} {
		workflow := readFlakeWorkflow(t, path)
		lintJob, ok := workflow.Jobs["lint"]
		if !ok {
			t.Fatalf("%s: no `lint` job", path)
		}

		var selfTestAt, runAt = -1, -1
		for i, step := range lintJob.Steps {
			switch {
			case step.Run == lintWrapperSelfTest:
				selfTestAt = i
			case step.Run == lintWrapperRun:
				runAt = i
			}
			// The raw invocation must not come back: it has no warm-up, no
			// cache reset and no retry, and its flakes are not verdicts.
			if strings.Contains(step.Run, "golangci-lint run") {
				t.Errorf("%s: step %q runs golangci-lint directly; use %s so the loader flake is retried instead of reddening the gate",
					path, step.Name, lintWrapperPath)
			}
		}
		if runAt < 0 {
			t.Fatalf("%s: no lint step runs %s", path, lintWrapperRun)
		}
		if selfTestAt < 0 {
			t.Fatalf("%s: the linter wrapper's hermetic self-test (%s) must run before it",
				path, lintWrapperSelfTest)
		}
		if selfTestAt > runAt {
			t.Errorf("%s: the self-test runs after the linter (self-test=%d, lint=%d)", path, selfTestAt, runAt)
		}
	}
}

// TestLintPinMatchesAcrossWorkflows: a PR's lint verdict and a release gate's
// lint verdict must come from the same linter build, otherwise a clean PR can
// gate a release that lints differently.
func TestLintPinMatchesAcrossWorkflows(t *testing.T) {
	ciPin := lintPin(t, readFlakeWorkflow(t, ciWorkflowFile))
	releasePin := lintPin(t, readFlakeWorkflow(t, releasePublishWorkflowFile))
	if ciPin != releasePin {
		t.Fatalf("golangci-lint pin differs: ci.yml %s, release-publish.yml %s", ciPin, releasePin)
	}
	if !strings.HasPrefix(ciPin, "v") {
		t.Fatalf("golangci-lint pin %q is not a version tag", ciPin)
	}
}

// TestLintWrapperOnlyRetriesTheLoaderFlake pins the classification boundary.
// The wrapper may retry a linter that could not load the package graph; it may
// never retry a run that reported findings, because that is what a red gate is
// for.
func TestLintWrapperOnlyRetriesTheLoaderFlake(t *testing.T) {
	raw, err := os.ReadFile(lintWrapperPath)
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)

	for _, want := range []struct{ needle, why string }{
		{"go build ./...", "a warm build cache and a compiler-level proof of the tree"},
		{"cache clean", "a clean linter cache before the first attempt"},
		{"could not load export data", "the export-data loader signature (PR #1337)"},
		{"goanalysis_metalinter", "the goanalysis_metalinter loader signature"},
		{"buildir: failed to load package", "the buildir loader signature"},
		{"(typecheck)", "the typecheck-cascade shape of the same broken load"},
		{"attempt", "an attempt number in the log, so a retried flake is visible"},
		{"failing closed", "fail-closed wording after the last attempt"},
		{"not retrying", "an explicit statement that findings are not retried"},
		{"--self-test", "its own hermetic regression suite"},
	} {
		if !strings.Contains(script, want.needle) {
			t.Errorf("%s does not reference %q (%s)", lintWrapperPath, want.needle, want.why)
		}
	}

	// Fail-closed: the exit code of the linter has to be propagated, not
	// collapsed into a generic 1 that hides a config error (exit 4/5) or a
	// loader failure (exit 3).
	if !strings.Contains(script, `return "$code"`) {
		t.Error("the wrapper must propagate golangci-lint's own exit code")
	}
}

// TestLintWrapperSelfTestRuns exercises the wrapper's decision branches for
// real. It is hermetic (stubbed go/golangci-lint on PATH, no network), so it
// runs on the ubuntu jobs where the wrapper itself runs.
func TestLintWrapperSelfTestRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the wrapper is a bash script and only runs on the Linux jobs")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	out, err := exec.Command("bash", lintWrapperPath, "--self-test").CombinedOutput()
	if err != nil {
		t.Fatalf("%s --self-test failed (%v):\n%s", lintWrapperPath, err, out)
	}
	if !strings.Contains(string(out), "0 failure(s)") {
		t.Fatalf("%s --self-test did not report a clean run:\n%s", lintWrapperPath, out)
	}
}

// TestReleasePleaseSerialisesItsRuns pins the duplicate-release fix.
//
// Two dispatches landed in the same second on 2026-11-13 (runs 43448561385 and
// 43448561447, both `repository_dispatch` on 7a65faad at 15:02:34Z), both ran
// release-please, and both created a GitHub release for v0.79.1: 444366819
// with the 12 real assets and 444366820 with none. GitHub resolves
// /releases/tags/v0.79.1 to the first, but lists (and /releases/latest before
// v0.80.0 superseded it) by id — so the empty object was "the newest release"
// and handed an empty asset list to anything that resolves the newest binary.
//
// A concurrency group makes the second run observe an already-merged release
// PR and an already-created release instead of racing it. cancel-in-progress
// must stay false: cancelling a run between `gh pr merge` and the rerun
// dispatch leaves the chain half-done, which is worse than waiting.
func TestReleasePleaseSerialisesItsRuns(t *testing.T) {
	workflow := readFlakeWorkflow(t, releasePleaseWorkflowFile)

	if strings.TrimSpace(workflow.Concurrency.Group) == "" {
		t.Fatal("release-please.yml has no workflow-level `concurrency` group; two dispatches can race a release creation (v0.79.1 duplicate, 2026-11-13)")
	}
	if !strings.Contains(workflow.Concurrency.Group, "github.repository") {
		t.Errorf("concurrency group %q should be scoped to the repository", workflow.Concurrency.Group)
	}
	if workflow.Concurrency.CancelInProgress == nil {
		t.Fatal("concurrency.cancel-in-progress must be set explicitly to false")
	}
	if *workflow.Concurrency.CancelInProgress {
		t.Fatal("concurrency.cancel-in-progress must be false: cancelling a release-please run between the release-PR merge and its rerun dispatch leaves the chain half-done")
	}
}

// TestPublishReleaseRejectsDuplicateReleaseObjects guards the second half of
// the duplicate defect: publishing resolves `/releases/tags/<tag>`, which
// silently picks one of two objects for a tag. The release guard already
// re-reads GitHub for the published assets; it must also prove the tag has
// exactly one release object, so a duplicate that still slips through is a
// loud failure instead of an /releases/latest trap.
func TestPublishReleaseRejectsDuplicateReleaseObjects(t *testing.T) {
	workflow := readFlakeWorkflow(t, releasePublishWorkflowFile)
	publishJob, ok := workflow.Jobs["publish-release"]
	if !ok {
		t.Fatal("release-publish.yml has no `publish-release` job")
	}

	verifyAt, publishAt := -1, -1
	for i, step := range publishJob.Steps {
		if step.Name == verifyAssetsStep {
			verifyAt = i
		}
		if step.Name == publishAssetsStepName {
			publishAt = i
		}
	}
	if verifyAt < 0 {
		t.Fatalf("%q step is missing", verifyAssetsStep)
	}
	if publishAt < 0 || verifyAt < publishAt {
		t.Fatalf("the release guard must run after %q (publish=%d, verify=%d)", publishAssetsStepName, publishAt, verifyAt)
	}

	script := publishJob.Steps[verifyAt].Run
	for _, want := range []struct{ needle, why string }{
		{"/releases?per_page=100", "a list of release objects, which is where a duplicate shows up"},
		{"tag_name", "filtering the list down to the tag being published"},
		{"-gt 1", "a hard failure when more than one object carries the tag"},
		{"/releases/latest", "the endpoint the duplicate can fool"},
	} {
		if !strings.Contains(script, want.needle) {
			t.Errorf("%q script does not reference %q (%s)", verifyAssetsStep, want.needle, want.why)
		}
	}
}
