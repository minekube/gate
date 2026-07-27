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
	Needs       string               `yaml:"needs"`
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

func repairJob(t *testing.T, workflow repairWorkflow, name string) repairWorkflowJob {
	t.Helper()

	job, ok := workflow.Jobs[name]
	if !ok {
		t.Fatalf("release-repair.yml has no %s job", name)
	}
	if len(job.Steps) == 0 {
		t.Fatalf("%s job has no steps", name)
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

	if len(workflow.Permissions) != 0 {
		t.Errorf("workflow permissions are %v; the repair path must have no ambient grant", workflow.Permissions)
	}

	build := repairJob(t, workflow, "build")
	if got := build.Permissions; len(got) != 1 || got["contents"] != "read" {
		t.Errorf("build job permissions are %v; tagged code must have contents: read only", got)
	}

	publish := repairJob(t, workflow, "publish")
	if got := publish.Permissions; len(got) != 1 || got["contents"] != "write" {
		t.Errorf("publish job permissions are %v; only the trusted publisher may upload release assets", got)
	}
	if publish.Needs != "build" {
		t.Errorf("publish job needs %q; it must consume the unprivileged build job", publish.Needs)
	}

	// packages: write is the specific grant that would make a ghcr push - and
	// therefore a backward :latest retag - possible.
	if strings.Contains(raw, "packages:") {
		t.Error("release-repair.yml mentions a packages: scope; a registry push must stay impossible")
	}

	// No credential can be smuggled in through an action either.
	for _, job := range []repairWorkflowJob{build, publish} {
		for _, step := range job.Steps {
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
	build := repairJob(t, workflow, "build")
	steps := build.Steps
	wantBuildSteps := []string{
		"Checkout the release tag",
		"Confirm the checkout is the tagged commit",
		"Refuse to repair a release that already has a build",
		"Setup Go from the tag's own go.mod",
		"Verify",
		"Confirm verification left the tagged tree unmodified",
		"Build the tag's release artifacts",
		"Stage the artifacts the manifest describes",
		"Upload staged release artifacts",
	}
	if len(steps) != len(wantBuildSteps) {
		t.Fatalf("build job has %d steps; expected %d", len(steps), len(wantBuildSteps))
	}
	for i, want := range wantBuildSteps {
		if steps[i].Name != want {
			t.Errorf("build step %d is %q; expected %q", i+1, steps[i].Name, want)
		}
	}

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

	for _, step := range repairJob(t, workflow, "publish").Steps {
		if strings.HasPrefix(step.Uses, "actions/checkout") {
			t.Errorf("publish step %q checks out the repository; it must run without tag-authored code", step.Name)
		}
		if strings.Contains(step.Run, "make test") || strings.HasPrefix(step.Uses, "goreleaser/") {
			t.Errorf("publish step %q executes tag-authored build code", step.Name)
		}
	}
}

func TestReleaseRepairScopesGitHubTokenToAPISteps(t *testing.T) {
	workflow, _ := readRepairWorkflow(t)
	build := repairJob(t, workflow, "build")
	publish := repairJob(t, workflow, "publish")

	for jobName, job := range map[string]repairWorkflowJob{"build": build, "publish": publish} {
		for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
			if _, ok := job.Env[name]; ok {
				t.Errorf("%s job exposes %s to every step; the token must be scoped to API steps", jobName, name)
			}
		}
	}

	checkoutAt := repairStepIndex(build.Steps, "Checkout the release tag")
	if checkoutAt < 0 {
		t.Fatal("build job has no checkout step")
	}
	if got := fmt.Sprint(build.Steps[checkoutAt].With["persist-credentials"]); got != "false" {
		t.Errorf("checkout persist-credentials is %q; tagged code must not inherit git credentials", got)
	}

	for _, name := range []string{"Verify", "Build the tag's release artifacts"} {
		at := repairStepIndex(build.Steps, name)
		if at < 0 {
			t.Fatalf("build job has no %q step", name)
		}
		for _, token := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
			if _, ok := build.Steps[at].Env[token]; ok {
				t.Errorf("tag-authored step %q exposes %s", name, token)
			}
		}
	}

	for _, target := range []struct {
		job  repairWorkflowJob
		name string
	}{
		{build, "Refuse to repair a release that already has a build"},
		{publish, "Refuse to repair a release that already has a build"},
		{publish, "Upload the missing assets"},
		{publish, "Verify published release assets"},
	} {
		at := repairStepIndex(target.job.Steps, target.name)
		if at < 0 {
			t.Fatalf("job has no %q step", target.name)
		}
		if got := target.job.Steps[at].Env["GH_TOKEN"]; got != "${{ secrets.GITHUB_TOKEN }}" {
			t.Errorf("API step %q has GH_TOKEN %q; it must use the scoped GITHUB_TOKEN secret", target.name, got)
		}
		if _, ok := target.job.Steps[at].Env["GITHUB_TOKEN"]; ok {
			t.Errorf("API step %q exposes GITHUB_TOKEN instead of step-scoped GH_TOKEN", target.name)
		}
	}
}

// TestReleaseRepairPublishesAssetsWithoutRewritingTheRelease pins how the
// assets land. These releases are years old; their notes, id and published-at
// date are the historical record and a repair must leave them untouched.
func TestReleaseRepairPublishesAssetsWithoutRewritingTheRelease(t *testing.T) {
	workflow, raw := readRepairWorkflow(t)
	build := repairJob(t, workflow, "build")
	publish := repairJob(t, workflow, "publish")
	buildSteps := build.Steps
	publishSteps := publish.Steps

	// GoReleaser's own release pipe would rewrite the release body, and on
	// tags predating `mode: keep-existing` it deletes and recreates the
	// release outright. Skipping publish keeps this workflow's only write to
	// the release the asset upload itself.
	goreleaserAt := -1
	for i, step := range buildSteps {
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
		t.Fatal("build job never runs GoReleaser; there would be nothing to upload")
	}

	stageAt := repairStepIndex(buildSteps, "Stage the artifacts the manifest describes")
	if stageAt < 0 {
		t.Fatal("build job has no artifact staging step")
	}
	stage := buildSteps[stageAt].Run
	if !strings.Contains(stage, "rm -rf upload") || !strings.Contains(stage, "mkdir -p upload") ||
		strings.Index(stage, "rm -rf upload") > strings.Index(stage, "mkdir -p upload") {
		t.Error("artifact staging must clear upload before recreating it")
	}

	artifactAt := repairStepIndex(buildSteps, "Upload staged release artifacts")
	if artifactAt < 0 {
		t.Fatal("build job never uploads the staged artifacts for the publish job")
	}
	artifact := buildSteps[artifactAt]
	if !strings.HasPrefix(artifact.Uses, "actions/upload-artifact@") {
		t.Errorf("staged artifact step uses %q; it must use actions/upload-artifact", artifact.Uses)
	}
	artifactPath := fmt.Sprint(artifact.With["path"])
	if !strings.Contains(artifactPath, "upload") || !strings.Contains(artifactPath, "dist/checksums.txt") {
		t.Errorf("staged artifact path is %q; it must include upload and dist/checksums.txt", artifactPath)
	}

	// softprops/action-gh-release PATCHes /releases/{id} to sync metadata
	// before uploading anything, which both rewrites historical notes and 403s
	// on releases like these. `gh release upload` only POSTs assets.
	if strings.Contains(raw, "softprops/action-gh-release") {
		t.Error("repair uses softprops/action-gh-release, which rewrites release metadata " +
			"before uploading; use gh release upload")
	}

	uploadAt := repairStepIndex(publishSteps, "Upload the missing assets")
	if uploadAt < 0 {
		t.Fatal("publish job has no upload step")
	}
	upload := publishSteps[uploadAt].Run
	if !strings.Contains(upload, "gh release upload") || !strings.Contains(upload, "--clobber") {
		t.Error("upload step must call gh release upload ... --clobber")
	}
	if strings.Contains(upload, "for f in upload/*") || !strings.Contains(upload, "MANIFEST") ||
		!strings.Contains(upload, "while read -r _ name") {
		t.Error("publish upload must derive its allowlist from checksums.txt, not glob the staged directory")
	}
	if !strings.Contains(upload, "unexpected file") {
		t.Error("publish upload must fail on artifact files outside the checksums allowlist")
	}
	nameCheck := strings.Index(upload, `name_base="$(basename -- "$name")"`)
	pathUse := strings.Index(upload, `file="$STAGED/$name"`)
	if nameCheck < 0 || pathUse < 0 || nameCheck > pathUse {
		t.Error("publish upload must validate manifest names before constructing staged paths")
	}
	for _, want := range []string{
		`[ -z "$name" ]`,
		`[ "$name" = "." ]`,
		`[ "$name" = ".." ]`,
		`[[ "$name" == *\\* ]]`,
		`[[ "$name" == -* ]]`,
		"invalid asset name",
	} {
		if !strings.Contains(upload, want) {
			t.Errorf("publish upload does not reject manifest name case %q", want)
		}
	}
	if !strings.Contains(upload, `gh release upload "$RELEASE_TAG" --clobber --repo "$GITHUB_REPOSITORY" --`) {
		t.Error("gh release upload must terminate options before the validated file list")
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
	buildSteps := repairJob(t, workflow, "build").Steps
	publishSteps := repairJob(t, workflow, "publish").Steps

	const (
		refuseStep = "Refuse to repair a release that already has a build"
		verifyStep = "Verify published release assets"
	)

	buildRefuseAt := repairStepIndex(buildSteps, refuseStep)
	if buildRefuseAt < 0 {
		t.Fatalf("build job is missing the %q guard; it could waste a full rebuild "+
			"consumers have already checksummed", refuseStep)
	}
	buildRefuse := buildSteps[buildRefuseAt].Run
	if buildSteps[buildRefuseAt].If != "" {
		t.Errorf("%q is conditional (if: %q); the guard must always run", refuseStep, buildSteps[buildRefuseAt].If)
	}
	// It must gate on real BUILD artifacts, not on the asset count: a release
	// carrying only a stray checksums.txt is still a hole worth repairing, and
	// one carrying a binary is not.
	if !strings.Contains(buildRefuse, "EXISTING_BUILDS") || !regexp.MustCompile(`EXISTING_BUILDS"\s*-gt\s*0`).MatchString(buildRefuse) {
		t.Errorf("%q does not refuse on a positive real-build count", refuseStep)
	}
	if !strings.Contains(buildRefuse, `^checksums\\.txt$`) {
		t.Errorf("%q does not classify metadata out of the build count", refuseStep)
	}

	publishRefuseAt := repairStepIndex(publishSteps, refuseStep)
	if publishRefuseAt < 0 {
		t.Fatalf("publish job is missing the %q guard; it could overwrite the bytes "+
			"would report success", verifyStep)
	}
	publishRefuse := publishSteps[publishRefuseAt].Run
	if publishSteps[publishRefuseAt].If != "" {
		t.Errorf("%q is conditional (if: %q); the guard must always run", refuseStep, publishSteps[publishRefuseAt].If)
	}
	if !strings.Contains(publishRefuse, "EXISTING_BUILDS") || !regexp.MustCompile(`EXISTING_BUILDS"\s*-gt\s*0`).MatchString(publishRefuse) {
		t.Errorf("%q does not refuse on a positive real-build count", refuseStep)
	}
	if !strings.Contains(publishRefuse, `^checksums\\.txt$`) {
		t.Errorf("%q does not classify metadata out of the build count", refuseStep)
	}

	uploadAt := repairStepIndex(publishSteps, "Upload the missing assets")
	if uploadAt < 0 || uploadAt != publishRefuseAt+1 {
		t.Errorf("publish guard must run immediately before upload (guard=%d, upload=%d)", publishRefuseAt, uploadAt)
	}

	verifyAt := repairStepIndex(publishSteps, verifyStep)
	if verifyAt < 0 {
		t.Fatalf("publish job is missing the %q step; a repair that uploaded nothing "+
			"would report success", verifyStep)
	}
	if publishSteps[verifyAt].If != "" {
		t.Errorf("%q is conditional (if: %q); the guard must always run", verifyStep, publishSteps[verifyAt].If)
	}

	// Same assertion ci.yml's releaser job makes, so a repaired release is
	// held to the release path's standard rather than a weaker one.
	verify := publishSteps[verifyAt].Run
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

	if publishRefuseAt > verifyAt {
		t.Error("the complete-release refusal runs after the landed-result verification")
	}
	// Last, so no later step can bypass it.
	if verifyAt != len(publishSteps)-1 {
		t.Errorf("%q is step %d of %d; it must be last so nothing can run after the guard",
			verifyStep, verifyAt+1, len(publishSteps))
	}
}
