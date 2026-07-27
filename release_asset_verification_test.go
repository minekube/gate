package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The release job used to end at "run GoReleaser and hope". A release whose
// upload silently produced nothing still looked green, because nothing ever
// re-read the release that actually landed. geyserlite shipped several such
// empty releases (and one carrying only a C header) before it grew this
// guard; these tests pin the same guard here.

const (
	verifyAssetsStepName  = "Verify published release assets"
	createReleaseStepName = "Run GoReleaser"
)

type assetWorkflow struct {
	Jobs map[string]assetWorkflowJob `yaml:"jobs"`
}

type assetWorkflowJob struct {
	Steps []assetWorkflowStep `yaml:"steps"`
}

type assetWorkflowStep struct {
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	If   string `yaml:"if"`
	Run  string `yaml:"run"`
}

func readReleaseJobSteps(t *testing.T) []assetWorkflowStep {
	t.Helper()

	// Gate publishes from the tag-gated releaser job in ci.yml rather than a
	// dedicated release.yml; the guard is API-side, so the publish mechanism
	// (GoReleaser here) is irrelevant to it.
	const workflowPath = ".github/workflows/ci.yml"

	workflowBytes, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}

	var workflow assetWorkflow
	if err := yaml.Unmarshal(workflowBytes, &workflow); err != nil {
		t.Fatal(err)
	}

	release, ok := workflow.Jobs["releaser"]
	if !ok {
		t.Fatal("releaser job is missing")
	}
	if len(release.Steps) == 0 {
		t.Fatal("releaser job has no steps")
	}

	return release.Steps
}

func stepIndex(steps []assetWorkflowStep, name string) int {
	for i, step := range steps {
		if step.Name == name {
			return i
		}
	}
	return -1
}

// TestReleaseVerifiesPublishedAssets is the core regression guard: the
// release job must fail when the published release carries no downloadable
// artifact.
func TestReleaseVerifiesPublishedAssets(t *testing.T) {
	steps := readReleaseJobSteps(t)

	verifyAt := stepIndex(steps, verifyAssetsStepName)
	if verifyAt < 0 {
		t.Fatalf("releaser job is missing the %q step; a release that publishes "+
			"no asset would ship green again", verifyAssetsStepName)
	}

	script := steps[verifyAt].Run
	if script == "" {
		t.Fatalf("%q must be a run step that asserts on the published release", verifyAssetsStepName)
	}
	if steps[verifyAt].If != "" {
		t.Errorf("%q is conditional (if: %q); the guard must always run",
			verifyAssetsStepName, steps[verifyAt].If)
	}
}

// TestReleaseVerificationReadsPublishedRelease is the point of the guard.
// Asserting against GoReleaser's own output would rebuild the exact "trust
// the run, not the artifact" defect one layer up, so the check has to go back
// to the GitHub API and read what actually landed.
func TestReleaseVerificationReadsPublishedRelease(t *testing.T) {
	steps := readReleaseJobSteps(t)

	verifyAt := stepIndex(steps, verifyAssetsStepName)
	if verifyAt < 0 {
		t.Fatalf("releaser job is missing the %q step", verifyAssetsStepName)
	}
	script := steps[verifyAt].Run

	for _, want := range []string{
		"/releases/tags/",    // re-reads the published release by tag
		"checksums.txt",      // the manifest every download path verifies against
		`"uploaded"`,         // only fully-uploaded assets count
		"releases/download/", // proves a build is actually served, not just listed
	} {
		if !strings.Contains(script, want) {
			t.Errorf("%q script does not reference %q; it must assert on the "+
				"published release, not on local build output", verifyAssetsStepName, want)
		}
	}

	// The guard must not be satisfied by inspecting the local dist/ directory:
	// those files exist even when the upload never happened.
	localOnly := !strings.Contains(script, "gh api")
	if localOnly {
		t.Error("verification must call the GitHub API to read the published release")
	}
}

// TestReleaseVerificationRequiresRealBuildArtifact pins the second failure
// condition. A non-empty asset list is not proof of a usable release: a
// release carrying only checksums.txt, signature bundles, SBOMs or a stray
// header has a positive asset count and still offers nothing anyone can run,
// so the guard must classify by name/type rather than count.
func TestReleaseVerificationRequiresRealBuildArtifact(t *testing.T) {
	steps := readReleaseJobSteps(t)

	verifyAt := stepIndex(steps, verifyAssetsStepName)
	if verifyAt < 0 {
		t.Fatalf("releaser job is missing the %q step", verifyAssetsStepName)
	}
	script := steps[verifyAt].Run

	// The metadata types that must NOT satisfy the guard on their own.
	for _, excluded := range []string{
		`^checksums\\.txt$`,        // the manifest itself
		`^SHA(256|512)SUMS$`,       // checksum manifests
		`^LICENSE`,                 // license metadata
		`^README`,                  // readme metadata
		`\\.sig$`,                  // detached signatures
		`\\.sigstore\\.json$`,      // signature bundles
		`\\.attest\\.spdx\\.json$`, // SBOM attestations
		`\\.spdx\\.json$`,          // SBOM metadata
		`\\.asc$`,                  // armored signatures
		`\\.pem$`,                  // certificate metadata
		`\\.sha256$`,               // checksum sidecars
		`\\.h$`,                    // C headers
		`\\.hpp$`,                  // C++ headers
		`\\.md$`,                   // markdown metadata
		`\\.txt$`,                  // text metadata
	} {
		if !strings.Contains(script, excluded) {
			t.Errorf("build-artifact classifier does not exclude %s; a release of pure "+
				"metadata would pass the guard", excluded)
		}
	}

	// And it must actually gate on the classified count, not just compute it.
	if !regexp.MustCompile(`BUILD_COUNT"\s*-eq\s*0`).MatchString(script) {
		t.Error("guard does not fail on the classified zero build-artifact count")
	}
}

// TestReleaseVerificationRunsAfterUpload pins the ordering. The guard is
// meaningless before the upload, and nothing that advertises the release may
// precede it.
func TestReleaseVerificationRunsAfterUpload(t *testing.T) {
	steps := readReleaseJobSteps(t)

	verifyAt := stepIndex(steps, verifyAssetsStepName)
	createAt := stepIndex(steps, createReleaseStepName)
	if verifyAt < 0 || createAt < 0 {
		t.Fatalf("expected both %q (%d) and %q (%d) in the releaser job",
			createReleaseStepName, createAt, verifyAssetsStepName, verifyAt)
	}
	if verifyAt < createAt {
		t.Errorf("%q runs before %q; it would always see an empty release",
			verifyAssetsStepName, createReleaseStepName)
	}
}
