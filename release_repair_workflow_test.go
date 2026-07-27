package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// release-repair.yml re-publishes the assets of an EXISTING release from that
// release's own tagged source. Two properties make that safe, and neither is
// self-evident from reading the file, so they are pinned here:
//
//  1. It cannot move a container pointer. ci.yml's build job pushes
//     ghcr.io/minekube/gate:latest; re-running it at an old tag would drag
//     :latest backwards and silently downgrade every puller. The repair path
//     holds no registry credential at all, so that is not "guarded against" -
//     it is unrepresentable. These tests fail the moment anything grants the
//     capability back.
//
//  2. It runs on the TAG's toolchain. `go-version-file: go.mod` resolves
//     against the checked-out tag, so an old release rebuilds under the Go it
//     shipped with. A newer Go's vet reports printf diagnostics the tag's own
//     toolchain never emitted (v0.48.0 picks up two, and `make test` runs
//     vet), which would turn a healthy release red for reasons that have
//     nothing to do with the release.

const repairWorkflowPath = ".github/workflows/release-repair.yml"

type repairWorkflow struct {
	On          repairTriggers               `yaml:"on"`
	Permissions map[string]string            `yaml:"permissions"`
	Jobs        map[string]repairWorkflowJob `yaml:"jobs"`
}

type repairTriggers struct {
	WorkflowDispatch struct {
		Inputs map[string]struct {
			Required bool `yaml:"required"`
		} `yaml:"inputs"`
	} `yaml:"workflow_dispatch"`
	Push     *yaml.Node `yaml:"push"`
	Schedule *yaml.Node `yaml:"schedule"`
}

type repairWorkflowJob struct {
	Permissions map[string]string    `yaml:"permissions"`
	Env         map[string]string    `yaml:"env"`
	Steps       []repairWorkflowStep `yaml:"steps"`
}

type repairWorkflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	If   string            `yaml:"if"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

func readRepairWorkflow(t *testing.T) (repairWorkflow, string) {
	t.Helper()

	raw, err := os.ReadFile(repairWorkflowPath)
	if err != nil {
		t.Fatalf("%s is missing; there is no credential-free way to repair a "+
			"release published without assets: %v", repairWorkflowPath, err)
	}

	var workflow repairWorkflow
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}

	return workflow, string(raw)
}

func repairJob(t *testing.T, workflow repairWorkflow) repairWorkflowJob {
	t.Helper()

	job, ok := workflow.Jobs["repair"]
	if !ok {
		t.Fatal("release-repair.yml has no repair job")
	}
	if len(job.Steps) == 0 {
		t.Fatal("repair job has no steps")
	}
	return job
}

func repairStepIndex(steps []repairWorkflowStep, name string) int {
	for i, step := range steps {
		if step.Name == name {
			return i
		}
	}
	return -1
}

// TestReleaseRepairGrantsNoRegistryScope is the capability boundary. The
// repair path must be structurally incapable of touching a registry, so that
// re-publishing a 2025 tag can never move the :latest users pull today.
func TestReleaseRepairGrantsNoRegistryScope(t *testing.T) {
	workflow, raw := readRepairWorkflow(t)

	// `contents: write` uploads release assets and nothing else. Any second
	// permission is a widened blast radius that has to be argued for.
	if got := workflow.Permissions; len(got) != 1 || got["contents"] != "write" {
		t.Errorf("workflow permissions are %v; the repair path must grant exactly "+
			"contents: write so a registry push is unrepresentable", got)
	}
	if perms := repairJob(t, workflow).Permissions; perms != nil {
		t.Errorf("repair job declares its own permissions %v; the single workflow-level "+
			"grant must be the whole story", perms)
	}

	// packages: write is the specific grant that would make a ghcr push - and
	// therefore a backward :latest retag - possible.
	if strings.Contains(raw, "packages:") {
		t.Error("release-repair.yml mentions a packages: scope; a registry push must stay impossible")
	}

	// No credential can be smuggled in through an action either.
	for _, step := range repairJob(t, workflow).Steps {
		for _, forbidden := range []string{
			"docker/login-action",
			"docker/build-push-action",
			"docker/setup-buildx-action",
		} {
			if strings.HasPrefix(step.Uses, forbidden) {
				t.Errorf("step %q uses %s; the repair path must not build or push images",
					step.Name, step.Uses)
			}
		}
		if strings.Contains(step.Run, "imagetools") || strings.Contains(step.Run, "docker push") {
			t.Errorf("step %q retags or pushes a container image; that capability is "+
				"deliberately not built here", step.Name)
		}
	}

	// GITHUB_TOKEN is the ambient, permission-scoped token; anything else is a
	// separately provisioned credential and would defeat the whole argument.
	for _, match := range regexp.MustCompile(`secrets\.([A-Za-z0-9_]+)`).FindAllStringSubmatch(raw, -1) {
		if match[1] != "GITHUB_TOKEN" {
			t.Errorf("release-repair.yml references secrets.%s; the repair path must run on "+
				"the scoped GITHUB_TOKEN alone", match[1])
		}
	}
}

// TestReleaseRepairRebuildsTheTagOnItsOwnToolchain pins the provenance
// properties: the assets must come from the tag's tree, built by the Go the
// tag declares.
func TestReleaseRepairRebuildsTheTagOnItsOwnToolchain(t *testing.T) {
	workflow, raw := readRepairWorkflow(t)
	steps := repairJob(t, workflow).Steps

	// A repair is dispatched by a human at an existing tag. It must not have a
	// push/schedule trigger that could fire it at anything else.
	if workflow.On.Push != nil || workflow.On.Schedule != nil {
		t.Error("release-repair.yml triggers on push or schedule; it must be manually dispatched only")
	}
	inputs := workflow.On.WorkflowDispatch.Inputs
	if len(inputs) != 1 {
		t.Errorf("workflow_dispatch inputs are %v; release_tag must be the only one, so no "+
			"input can ever weaken the rebuild", inputs)
	}
	if tag, ok := inputs["release_tag"]; !ok || !tag.Required {
		t.Error("release_tag input is missing or not required")
	}

	if !strings.Contains(raw, "ref: ${{ inputs.release_tag }}") {
		t.Error("checkout does not pin ref to the dispatched tag; a repair would build a branch")
	}

	// THE PIN. `go-version-file: go.mod` reads the go.mod of the CHECKED-OUT
	// TAG, so the rebuild runs on the tag's own clock. A hardcoded go-version
	// here would be today's Go applied to old source.
	setupAt := -1
	for i, step := range steps {
		if strings.HasPrefix(step.Uses, "actions/setup-go") {
			setupAt = i
			if got := fmt.Sprint(step.With["go-version-file"]); got != "go.mod" {
				t.Errorf("setup-go uses go-version-file %q; it must be go.mod so the tag "+
					"rebuilds under the Go it shipped with", got)
			}
			if _, pinned := step.With["go-version"]; pinned {
				t.Error("setup-go hardcodes go-version; that applies today's Go - and today's " +
					"vet diagnostics - to old tagged source")
			}
		}
	}
	if setupAt < 0 {
		t.Fatal("repair job never sets up Go")
	}

	// The tag's own verification runs, unconditionally. A mechanism that can
	// skip a tag's tests is a durable supply-chain capability that will be
	// reached for again, by someone with less context, on a tag whose failing
	// test does test software.
	verifyAt := repairStepIndex(steps, "Verify")
	if verifyAt < 0 {
		t.Fatal("repair job does not run the tag's own verification")
	}
	if !strings.Contains(steps[verifyAt].Run, "make test") {
		t.Errorf("Verify step runs %q, not make test", steps[verifyAt].Run)
	}
	if steps[verifyAt].If != "" {
		t.Errorf("Verify is conditional (if: %q); the tag's tests must always run", steps[verifyAt].If)
	}
	if verifyAt < setupAt {
		t.Error("Verify runs before Go is set up")
	}
	for _, bypass := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)skip[-_](test|tests|ci|verify|checks)`),
		regexp.MustCompile(`(?i)build[-_]only`),
	} {
		if bypass.MatchString(raw) {
			t.Errorf("release-repair.yml contains a test-bypass affordance matching %s", bypass)
		}
	}
}

func TestReleaseRepairScopesGitHubTokenToAPISteps(t *testing.T) {
	workflow, _ := readRepairWorkflow(t)
	job := repairJob(t, workflow)
	steps := job.Steps

	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if _, ok := job.Env[name]; ok {
			t.Errorf("repair job exposes %s to every step; the token must be scoped to API steps", name)
		}
	}

	checkoutAt := repairStepIndex(steps, "Checkout the release tag")
	if checkoutAt < 0 {
		t.Fatal("repair job has no checkout step")
	}
	if got := fmt.Sprint(steps[checkoutAt].With["persist-credentials"]); got != "false" {
		t.Errorf("checkout persist-credentials is %q; tagged code must not inherit git credentials", got)
	}

	for _, name := range []string{"Verify", "Build the tag's release artifacts"} {
		at := repairStepIndex(steps, name)
		if at < 0 {
			t.Fatalf("repair job has no %q step", name)
		}
		for _, token := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
			if _, ok := steps[at].Env[token]; ok {
				t.Errorf("tag-authored step %q exposes %s", name, token)
			}
		}
	}

	for _, name := range []string{
		"Refuse to repair a release that already has a build",
		"Upload the missing assets",
		"Verify published release assets",
	} {
		at := repairStepIndex(steps, name)
		if at < 0 {
			t.Fatalf("repair job has no %q step", name)
		}
		if got := steps[at].Env["GH_TOKEN"]; got != "${{ secrets.GITHUB_TOKEN }}" {
			t.Errorf("API step %q has GH_TOKEN %q; it must use the scoped GITHUB_TOKEN secret", name, got)
		}
		if _, ok := steps[at].Env["GITHUB_TOKEN"]; ok {
			t.Errorf("API step %q exposes GITHUB_TOKEN instead of step-scoped GH_TOKEN", name)
		}
	}
}

// TestReleaseRepairPublishesAssetsWithoutRewritingTheRelease pins how the
// assets land. These releases are years old; their notes, id and published-at
// date are the historical record and a repair must leave them untouched.
func TestReleaseRepairPublishesAssetsWithoutRewritingTheRelease(t *testing.T) {
	workflow, raw := readRepairWorkflow(t)
	steps := repairJob(t, workflow).Steps

	// GoReleaser's own release pipe would rewrite the release body, and on
	// tags predating `mode: keep-existing` it deletes and recreates the
	// release outright. Skipping publish keeps this workflow's only write to
	// the release the asset upload itself.
	goreleaserAt := -1
	for i, step := range steps {
		if strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action") {
			goreleaserAt = i
			args := fmt.Sprint(step.With["args"])
			if !strings.Contains(args, "--skip=publish") {
				t.Errorf("GoReleaser args %q do not skip publish; the repair would rewrite "+
					"the historical release's notes and identity", args)
			}
			if strings.Contains(args, "validate") {
				t.Errorf("GoReleaser args %q skip validation; the rebuild must stay bound to "+
					"the clean tagged tree", args)
			}
		}
	}
	if goreleaserAt < 0 {
		t.Fatal("repair job never runs GoReleaser; there would be nothing to upload")
	}

	// softprops/action-gh-release PATCHes /releases/{id} to sync metadata
	// before uploading anything, which both rewrites historical notes and 403s
	// on releases like these. `gh release upload` only POSTs assets.
	if strings.Contains(raw, "softprops/action-gh-release") {
		t.Error("repair uses softprops/action-gh-release, which rewrites release metadata " +
			"before uploading; use gh release upload")
	}

	uploadAt := repairStepIndex(steps, "Upload the missing assets")
	if uploadAt < 0 {
		t.Fatal("repair job has no upload step")
	}
	upload := steps[uploadAt].Run
	if !strings.Contains(upload, "gh release upload") || !strings.Contains(upload, "--clobber") {
		t.Error("upload step must call gh release upload ... --clobber")
	}
	if uploadAt < goreleaserAt {
		t.Error("assets are uploaded before they are built")
	}

	// --clobber is a blunt instrument. It may only ever be handed names that
	// are holes, so an asset already published, uploaded and non-empty is
	// kept rather than overwritten.
	if !strings.Contains(upload, `.state == "uploaded" and .size > 0`) {
		t.Error("upload step does not exclude already-good assets from --clobber; a healthy " +
			"asset could be overwritten with a fresh build of the same name")
	}
}

// TestReleaseRepairRefusesCompleteReleasesAndVerifiesTheLandedResult pins the
// two guards that bracket the upload: repair only fills holes, and it never
// trusts its own upload step.
func TestReleaseRepairRefusesCompleteReleasesAndVerifiesTheLandedResult(t *testing.T) {
	workflow, _ := readRepairWorkflow(t)
	steps := repairJob(t, workflow).Steps

	const (
		refuseStep = "Refuse to repair a release that already has a build"
		verifyStep = "Verify published release assets"
	)

	refuseAt := repairStepIndex(steps, refuseStep)
	if refuseAt < 0 {
		t.Fatalf("repair job is missing the %q guard; it could overwrite the bytes "+
			"consumers have already checksummed", refuseStep)
	}
	refuse := steps[refuseAt].Run
	if steps[refuseAt].If != "" {
		t.Errorf("%q is conditional (if: %q); the guard must always run", refuseStep, steps[refuseAt].If)
	}
	// It must gate on real BUILD artifacts, not on the asset count: a release
	// carrying only a stray checksums.txt is still a hole worth repairing, and
	// one carrying a binary is not.
	if !strings.Contains(refuse, "EXISTING_BUILDS") || !regexp.MustCompile(`EXISTING_BUILDS"\s*-gt\s*0`).MatchString(refuse) {
		t.Errorf("%q does not refuse on a positive real-build count", refuseStep)
	}
	if !strings.Contains(refuse, `^checksums\\.txt$`) {
		t.Errorf("%q does not classify metadata out of the build count", refuseStep)
	}

	verifyAt := repairStepIndex(steps, verifyStep)
	if verifyAt < 0 {
		t.Fatalf("repair job is missing the %q step; a repair that uploaded nothing "+
			"would report success", verifyStep)
	}
	if steps[verifyAt].If != "" {
		t.Errorf("%q is conditional (if: %q); the guard must always run", verifyStep, steps[verifyAt].If)
	}

	// Same assertion ci.yml's releaser job makes, so a repaired release is
	// held to the release path's standard rather than a weaker one.
	verify := steps[verifyAt].Run
	for _, want := range []string{
		"/releases/tags/",    // re-reads the PUBLISHED release, not local dist/
		"gh api",             // ... over the API
		"checksums.txt",      // the manifest every download path resolves
		`"uploaded"`,         // only fully-uploaded assets count
		"releases/download/", // proves a build is served, not merely listed
	} {
		if !strings.Contains(verify, want) {
			t.Errorf("%q does not reference %q; it must assert on the landed release, "+
				"not on the upload step's exit code", verifyStep, want)
		}
	}
	if !regexp.MustCompile(`BUILD_COUNT"\s*-eq\s*0`).MatchString(verify) {
		t.Errorf("%q does not fail on a zero real-build count; a release of pure metadata "+
			"would pass", verifyStep)
	}

	if refuseAt > verifyAt {
		t.Error("the complete-release refusal runs after the landed-result verification")
	}
	// Last, so no later step can bypass it.
	if verifyAt != len(steps)-1 {
		t.Errorf("%q is step %d of %d; it must be last so nothing can run after the guard",
			verifyStep, verifyAt+1, len(steps))
	}
}
