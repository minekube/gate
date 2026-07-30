package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Tagged release code runs in release-publish.yml with read-only credentials;
// only its fresh publisher jobs may receive write scopes.

const ciWorkflowPath = ".github/workflows/ci.yml"
const trustedReleaseWorkflowPath = ".github/workflows/release-publish.yml"

type ciIsolationWorkflow struct {
	Permissions map[string]string                 `yaml:"permissions"`
	Jobs        map[string]ciIsolationWorkflowJob `yaml:"jobs"`
}

type ciIsolationWorkflowJob struct {
	Needs       ciIsolationNeeds          `yaml:"needs"`
	Permissions map[string]string         `yaml:"permissions"`
	Steps       []ciIsolationWorkflowStep `yaml:"steps"`
}

type ciIsolationWorkflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

type ciIsolationNeeds []string

func (n *ciIsolationNeeds) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*n = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		var needs []string
		if err := node.Decode(&needs); err != nil {
			return err
		}
		*n = needs
		return nil
	default:
		return fmt.Errorf("needs must be a string or list, got YAML kind %d", node.Kind)
	}
}

func readCIIsolationWorkflow(t *testing.T) (ciIsolationWorkflow, string) {
	return readCIIsolationWorkflowAt(t, ciWorkflowPath)
}

func readCIIsolationWorkflowAt(t *testing.T, path string) (ciIsolationWorkflow, string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var workflow ciIsolationWorkflow
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow, string(raw)
}

func ciIsolationJob(t *testing.T, workflow ciIsolationWorkflow, name string) ciIsolationWorkflowJob {
	t.Helper()

	job, ok := workflow.Jobs[name]
	if !ok {
		t.Fatalf("workflow has no %q job", name)
	}
	if len(job.Steps) == 0 {
		t.Fatalf("job %q has no steps", name)
	}
	return job
}

func ciIsolationStepIndex(steps []ciIsolationWorkflowStep, name string) int {
	for i, step := range steps {
		if step.Name == name {
			return i
		}
	}
	return -1
}

func TestCIGrantsNoAmbientWritePermission(t *testing.T) {
	workflow, _ := readCIIsolationWorkflow(t)
	if len(workflow.Permissions) != 0 {
		t.Fatalf("workflow permissions are %v; ci.yml must have no ambient token grant", workflow.Permissions)
	}

	for _, name := range []string{"lint", "test", "docker-smoke"} {
		job := ciIsolationJob(t, workflow, name)
		if got := job.Permissions; len(got) != 1 || got["contents"] != "read" {
			t.Errorf("%s permissions are %v; code-running jobs must have contents: read only", name, got)
		}
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/checkout@") &&
				fmt.Sprint(step.With["persist-credentials"]) != "false" {
				t.Errorf("%s checkout %q persists credentials; tagged code could recover the token",
					name, step.Name)
			}
			for _, token := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
				if _, ok := step.Env[token]; ok {
					t.Errorf("%s step %q exposes %s to a code-running job", name, step.Name, token)
				}
			}
		}
	}

	trusted, _ := readCIIsolationWorkflowAt(t, trustedReleaseWorkflowPath)
	if len(trusted.Permissions) != 0 {
		t.Fatalf("trusted release workflow permissions are %v; it must have no ambient token grant", trusted.Permissions)
	}
	for _, name := range []string{"lint", "test", "docker-smoke", "image-build", "release-build"} {
		job := ciIsolationJob(t, trusted, name)
		if got := job.Permissions; len(got) != 1 || got["contents"] != "read" {
			t.Errorf("%s permissions are %v; tagged code must have contents: read only", name, got)
		}
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/checkout@") &&
				fmt.Sprint(step.With["persist-credentials"]) != "false" {
				t.Errorf("%s checkout %q persists credentials; tagged code could recover the token",
					name, step.Name)
			}
			for _, token := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
				if _, ok := step.Env[token]; ok {
					t.Errorf("%s step %q exposes %s to a code-running job", name, step.Name, token)
				}
			}
		}
	}
}

func TestReleasePublishUsesTrustedDefaultBranchWorkflow(t *testing.T) {
	releasePlease, err := os.ReadFile(".github/workflows/release-please.yml")
	if err != nil {
		t.Fatal(err)
	}

	contents := string(releasePlease)
	if !strings.Contains(contents, "gh workflow run release-publish.yml") {
		t.Fatal("release-please must dispatch the trusted release-publish workflow")
	}
	if !strings.Contains(contents, "--ref master") {
		t.Fatal("release-please must dispatch release-publish.yml from master")
	}
	if strings.Contains(contents, "gh workflow run ci.yml") {
		t.Fatal("release-please must not dispatch the tag-selected ci.yml workflow")
	}

	workflow, err := os.ReadFile(".github/workflows/release-publish.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(workflow), "release_tag:") {
		t.Fatal("release-publish.yml must receive the release tag as an input")
	}
}

func TestCISeparatesContainerBuildFromRegistryWrite(t *testing.T) {
	workflow, _ := readCIIsolationWorkflowAt(t, trustedReleaseWorkflowPath)
	build := ciIsolationJob(t, workflow, "image-build")
	publish := ciIsolationJob(t, workflow, "publish-images")

	if got := publish.Permissions; len(got) != 2 ||
		got["actions"] != "read" || got["packages"] != "write" {
		t.Errorf("container publisher permissions are %v; expected actions: read and packages: write only", got)
	}
	if len(publish.Needs) != 1 || publish.Needs[0] != "image-build" {
		t.Errorf("container publisher needs %v; it must consume only image-build", publish.Needs)
	}

	buildRaw := fmt.Sprint(build.Steps)
	if strings.Contains(buildRaw, "docker/login-action") || strings.Contains(buildRaw, "push:true") {
		t.Error("image-build can authenticate or push; tagged Dockerfile code must run without registry write access")
	}
	if !strings.Contains(buildRaw, "type=oci") || !strings.Contains(buildRaw, "actions/upload-artifact@") {
		t.Error("image-build does not hand OCI archives to the fresh publisher job")
	}

	publishRaw := fmt.Sprint(publish.Steps)
	if strings.Contains(publishRaw, "actions/checkout@") ||
		strings.Contains(publishRaw, "docker/build-push-action@") {
		t.Error("container publisher checks out or builds tagged code instead of consuming staged images")
	}
	for _, want := range []string{
		"actions/download-artifact@",
		"docker/login-action@",
		"oras cp --from-oci-layout",
		"unexpected file",
	} {
		if !strings.Contains(publishRaw, want) {
			t.Errorf("container publisher does not contain %q", want)
		}
	}
}

func TestCISeparatesReleaseBuildFromContentsWrite(t *testing.T) {
	workflow, _ := readCIIsolationWorkflowAt(t, trustedReleaseWorkflowPath)
	build := ciIsolationJob(t, workflow, "release-build")
	publish := ciIsolationJob(t, workflow, "publish-release")

	if got := publish.Permissions; len(got) != 2 ||
		got["actions"] != "read" || got["contents"] != "write" {
		t.Errorf("release publisher permissions are %v; expected actions: read and contents: write only", got)
	}
	if len(publish.Needs) != 1 || publish.Needs[0] != "release-build" {
		t.Errorf("release publisher needs %v; it must consume only release-build", publish.Needs)
	}

	goreleaserAt := -1
	for i, step := range build.Steps {
		if strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action@") {
			goreleaserAt = i
			args := fmt.Sprint(step.With["args"])
			if !strings.Contains(args, "--skip=publish") {
				t.Errorf("release build GoReleaser args %q do not disable publication", args)
			}
			if _, ok := step.Env["GITHUB_TOKEN"]; ok {
				t.Error("release build exposes GITHUB_TOKEN to GoReleaser")
			}
		}
	}
	if goreleaserAt < 0 {
		t.Fatal("release-build never runs GoReleaser")
	}

	buildRaw := fmt.Sprint(build.Steps)
	if !strings.Contains(buildRaw, "checksums.txt") ||
		!strings.Contains(buildRaw, "actions/upload-artifact@") {
		t.Error("release-build does not stage the checksums allowlist and hand it to the publisher")
	}

	publishRaw := fmt.Sprint(publish.Steps)
	if strings.Contains(publishRaw, "actions/checkout@") ||
		strings.Contains(publishRaw, "goreleaser/goreleaser-action@") {
		t.Error("release publisher checks out or executes tagged release code")
	}
	for _, want := range []string{
		"actions/download-artifact@",
		"gh release upload",
		"while read -r _ name",
		"unexpected file",
	} {
		if !strings.Contains(publishRaw, want) {
			t.Errorf("release publisher does not contain %q", want)
		}
	}

	verifyAt := ciIsolationStepIndex(publish.Steps, verifyAssetsStepName)
	if verifyAt < 0 {
		t.Fatalf("release publisher is missing %q", verifyAssetsStepName)
	}
	if verifyAt != len(publish.Steps)-1 {
		t.Errorf("%q is step %d of %d; post-publish verification must remain last",
			verifyAssetsStepName, verifyAt+1, len(publish.Steps))
	}
}
