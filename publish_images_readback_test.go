package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// `trigger-release / publish-images` used to be two bare `oras cp` calls, and
// it failed twice in 75 minutes on 2026-11-13 while `publish-release` (the
// GitHub release assets) succeeded:
//
//	v0.79.0  run 43444043123  job 131960268990  digest 7bb3450419fa
//	v0.80.1  run 43454809602  job 131992784822  digest 451d1dac4ca4
//
//	Copying 451d1dac4ca4 application/vnd.oci.image.manifest.v1+json
//	Copying 50d5c85cd056 application/vnd.oci.image.layer.v1.tar+gzip
//	Copied  aadd31a61a73 application/vnd.oci.image.layer.v1.tar+gzip
//	Error response from registry: sha256:451d1dac...: not found
//
// Root cause: after pushing a manifest, oras 1.3.0 re-reads that same manifest
// from the DESTINATION to print the status line for its successors
// (cmd/oras/internal/display/status.TextCopyHandler.PostCopy ->
// graph.FilteredSuccessors(dst)). The registry had not yet made the manifest it
// had just accepted readable, so the copy aborted after the child landed and
// before the top-level index was pushed - the release was published with no
// image (v0.80.1 has no ghcr tag in either minekube/gate or minekube/gate/jre;
// both failing manifests exist in the registry today while neither release
// index b84cb7ccfe9e / b61fec4aadd5 was ever pushed). A red publish-images run
// also skips dispatch-moxy-bump, so the release is inert for production.
//
// The step therefore retries ONLY that registry read-after-write class, a
// bounded number of times (the copy is idempotent), attempts both images, and
// then a separate step proves the published tags resolve to the staged digests
// with both platform manifests readable. These tests execute the workflow's own
// step shells with a stubbed oras so a future edit cannot quietly turn a real
// failure into a retry, drop the verification, or lose the bound.

const (
	publishImagesStepName = "Publish staged container images"
	verifyImagesStepName  = "Verify published container images"

	// 64-hex digests used by every case; the staged digests differ per image so
	// a verification that compares against the wrong one is caught.
	stubGateDigest    = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	stubGateJREDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	stubTagSHA        = "470e11a906add25a679a246e1f0bf5dfdaea0253"
)

// stubOras answers the publish step the way the registry behaved in each case.
// Attempts are counted per staged layout through $STUB_CALLS.
const stubOras = `#!/usr/bin/env bash
printf '%s\n' "$*" >>"$STUB_CALLS"
expected="$STUB_GATE_DIGEST"
case "$*" in *"/jre:"*) expected="$STUB_GATE_JRE_DIGEST" ;; esac
case "$STUB_MODE" in
  ok)
    if [[ "$*" == resolve* ]]; then echo "$expected"; exit 0; fi
    exit 0 ;;
  race-once)
    if [[ "$*" == *gate-image.tar* ]] && [ "$(grep -c 'gate-image.tar' "$STUB_CALLS")" -eq 1 ]; then
      echo "Copying 451d1dac4ca4 application/vnd.oci.image.manifest.v1+json"
      echo "Copying 50d5c85cd056 application/vnd.oci.image.layer.v1.tar+gzip"
      echo "Error response from registry: sha256:451d1dac4ca4de4d478db0ce754a0421cf157e2881b947a086e916008147309c: not found"
      exit 1
    fi
    exit 0 ;;
  race-always)
    echo "Error response from registry: sha256:7bb3450419fa83c7d70e55e20ff6ae000c7fe190538c773591e1d61d51908f44: not found"
    exit 1 ;;
  source-missing)
    echo 'Error from source oci-layout for "release-images/gate-image.tar": sha256:ac790b62028e8a1b0ce6a55ef8aa066e967da671d19f3d147dee6732cf10882f: application/vnd.oci.image.layer.v1.tar+gzip: not found'
    exit 1 ;;
  unauthorized)
    echo "Error response from registry: unauthorized: authentication required"
    exit 1 ;;
  resolve-wrong) echo "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" ;;
  platform-missing)
    if [[ "$*" == *"--platform linux/arm64"* ]]; then
      echo "Error response from registry: failed to fetch the content of the index" >&2
      exit 1
    fi
    if [[ "$*" == resolve* ]]; then echo "$expected"; exit 0; fi
    echo '{"schemaVersion":2}' ;;
  *) if [[ "$*" == resolve* ]]; then echo "$expected"; exit 0; fi; exit 0 ;;
esac
`

// publishStepRunBlock returns the shell of one publish-images step.
func publishStepRunBlock(t *testing.T, stepName string) string {
	t.Helper()

	workflow, _ := readCIIsolationWorkflowAt(t, trustedReleaseWorkflowPath)
	job := ciIsolationJob(t, workflow, "publish-images")
	index := ciIsolationStepIndex(job.Steps, stepName)
	if index < 0 {
		t.Fatalf("publish-images has no %q step", stepName)
	}
	if strings.TrimSpace(job.Steps[index].Run) == "" {
		t.Fatalf("publish-images step %q is not a shell step", stepName)
	}
	return job.Steps[index].Run
}

// runPublishStep executes a step's shell exactly as the runner does: a fresh
// workspace with the downloaded container artifact and a stubbed oras/sleep.
func runPublishStep(t *testing.T, stepName, mode string) (string, []string, int) {
	t.Helper()

	// The step only ever runs on ubuntu-latest; windows has no bash here and is
	// skipped the same way TestLintWrapperSelfTestRuns skips its wrapper.
	if runtime.GOOS == "windows" {
		t.Skip("the container publisher is a bash step and only runs on the Linux jobs")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}

	dir := t.TempDir()
	layout := filepath.Join(dir, "release-images")
	if err := os.MkdirAll(layout, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"gate-image.tar", "gate-jre-image.tar"} {
		if err := os.WriteFile(filepath.Join(layout, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "oras"), []byte(stubOras), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "sleep"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(dir, "step.sh")
	if err := os.WriteFile(script, []byte(publishStepRunBlock(t, stepName)), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STUB_MODE="+mode,
		"STUB_CALLS="+filepath.Join(dir, "oras.calls"),
		"STUB_GATE_DIGEST="+stubGateDigest,
		"STUB_GATE_JRE_DIGEST="+stubGateJREDigest,
		"REGISTRY=ghcr.io",
		"GITHUB_REPOSITORY=minekube/gate",
		"GATE_DIGEST="+stubGateDigest,
		"GATE_JRE_DIGEST="+stubGateJREDigest,
		"IMAGE_TAG=470e11a",
		"IMAGE_TAG_SHA="+stubTagSHA,
		"RELEASE_TAG_SHA="+stubTagSHA,
		"VERIFIED_TAG_SHA="+stubTagSHA,
		"RELEASE_TAG=v0.80.1",
		"PUBLISH_RETRY_DELAY=1",
		"PUBLISH_ATTEMPTS="+publishAttemptBudget,
	)
	output, err := cmd.CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running %q step: %v\n%s", stepName, err, output)
	}

	// Every oras invocation the step made, in order.
	var orasCalls []string
	if calls, readErr := os.ReadFile(filepath.Join(dir, "oras.calls")); readErr == nil {
		for _, line := range strings.Split(string(calls), "\n") {
			if strings.TrimSpace(line) != "" {
				orasCalls = append(orasCalls, line)
			}
		}
	}
	return string(output), orasCalls, code
}

const publishAttemptBudget = "3"

func TestPublishImagesRetriesOnlyTheRegistryReadAfterWriteRace(t *testing.T) {
	output, _, code := runPublishStep(t, publishImagesStepName, "race-once")
	if code != 0 {
		t.Fatalf("a copy that lost the read-after-write race once must be retried to success, got exit %d:\n%s", code, output)
	}
	if !strings.Contains(output, "Published gate (ghcr.io/minekube/gate) on attempt 2.") {
		t.Errorf("retried gate copy was not reported as attempt 2:\n%s", output)
	}
	if !strings.Contains(output, "::warning::gate: attempt 1/"+publishAttemptBudget+" lost the registry read-after-write race") {
		t.Errorf("the retry must be announced with its attempt budget:\n%s", output)
	}
	if !strings.Contains(output, "Published gate-jre (ghcr.io/minekube/gate/jre) on attempt 1.") {
		t.Errorf("the JRE copy must still run and succeed:\n%s", output)
	}
	if strings.Count(output, "::warning::") != 1 {
		t.Errorf("exactly one retry was expected:\n%s", output)
	}
}

func TestPublishImagesFailsClosedOnANonTransientFailure(t *testing.T) {
	for _, mode := range []string{"source-missing", "unauthorized"} {
		t.Run(mode, func(t *testing.T) {
			output, calls, code := runPublishStep(t, publishImagesStepName, mode)
			if code == 0 {
				t.Fatalf("%s must fail the step:\n%s", mode, output)
			}
			if !strings.Contains(output, "failed with a non-transient error; not retrying.") {
				t.Errorf("%s was not classified as non-transient:\n%s", mode, output)
			}
			if strings.Contains(output, "::warning::") {
				t.Errorf("%s must not be retried:\n%s", mode, output)
			}
			// The JRE image is still attempted: the two renders are independent,
			// so a re-run only has to repair the missing one.
			if !strings.Contains(output, "Published container images are incomplete") {
				t.Errorf("%s must still report the incomplete publish:\n%s", mode, output)
			}
			var copies int
			for _, call := range calls {
				if strings.HasPrefix(call, "cp ") {
					copies++
				}
			}
			if copies != 2 {
				t.Errorf("%s made %d oras cp calls; want exactly one attempt per image", mode, copies)
			}
		})
	}
}

func TestPublishImagesRetryIsBounded(t *testing.T) {
	output, _, code := runPublishStep(t, publishImagesStepName, "race-always")
	if code == 0 {
		t.Fatalf("a copy that never reaches the registry must fail:\n%s", output)
	}
	if !strings.Contains(output, "still hit the registry read-after-write race after "+publishAttemptBudget+" attempts.") {
		t.Errorf("bounded retry did not report its exhausted budget:\n%s", output)
	}
	if strings.Count(output, "::warning::") != 2*2 {
		t.Errorf("want two retry warnings per image (%s), got:\n%s", publishAttemptBudget, output)
	}
	if !strings.Contains(output, "Published container images are incomplete") {
		t.Errorf("exhausted retries must fail closed:\n%s", output)
	}
}

func TestVerifyPublishedImagesIsFailClosed(t *testing.T) {
	output, _, code := runPublishStep(t, verifyImagesStepName, "ok")
	if code != 0 {
		t.Fatalf("a complete publication must verify, exit %d:\n%s", code, output)
	}
	if !strings.Contains(output, "Verified gate and gate/jre") {
		t.Errorf("verification did not report success:\n%s", output)
	}

	output, _, code = runPublishStep(t, verifyImagesStepName, "resolve-wrong")
	if code == 0 {
		t.Fatalf("a tag resolving to another digest must fail verification:\n%s", output)
	}
	if !strings.Contains(output, "expected the staged digest") {
		t.Errorf("wrong-digest failure is not explained:\n%s", output)
	}

	output, _, code = runPublishStep(t, verifyImagesStepName, "platform-missing")
	if code == 0 {
		t.Fatalf("a missing platform manifest must fail verification:\n%s", output)
	}
	if !strings.Contains(output, "no readable linux/arm64 manifest") {
		t.Errorf("missing-platform failure is not explained:\n%s", output)
	}
}

// TestPublishImagesKeepsBothStepsAndTheirOrder pins the wiring: the copy must
// stay behind the artifact/permission guards, and the verification must run
// after the copy (a verification before it proves nothing).
func TestPublishImagesKeepsBothStepsAndTheirOrder(t *testing.T) {
	workflow, _ := readCIIsolationWorkflowAt(t, trustedReleaseWorkflowPath)
	job := ciIsolationJob(t, workflow, "publish-images")

	publish := ciIsolationStepIndex(job.Steps, publishImagesStepName)
	verify := ciIsolationStepIndex(job.Steps, verifyImagesStepName)
	if publish < 0 || verify < 0 {
		t.Fatalf("publish-images must keep both %q and %q", publishImagesStepName, verifyImagesStepName)
	}
	if verify < publish {
		t.Error("the image verification must run after the copy")
	}

	run := job.Steps[publish].Run
	for _, want := range []string{
		"oras cp --from-oci-layout",
		"is_read_after_write_race",
		"not retrying",
		"publish_staged_image release-images/gate-image.tar",
		"publish_staged_image release-images/gate-jre-image.tar",
		"unexpected file",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("publish step lost %q", want)
		}
	}

	verifyRun := job.Steps[verify].Run
	for _, want := range []string{
		"oras resolve",
		"oras manifest fetch --platform",
		"linux/amd64 linux/arm64",
		"exit 1",
	} {
		if !strings.Contains(verifyRun, want) {
			t.Errorf("verify step lost %q", want)
		}
	}

	// The verification may not be skippable and the job keeps its scopes.
	if strings.Contains(verifyRun, "continue-on-error") || strings.Contains(verifyRun, "|| true") {
		t.Error("the verification must not be able to pass without checking the images")
	}
	if got := job.Permissions; len(got) != 2 || got["actions"] != "read" || got["packages"] != "write" {
		t.Errorf("container publisher permissions are %v; expected actions: read and packages: write only", got)
	}
}
