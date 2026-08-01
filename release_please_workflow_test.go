package main

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const approvedDispatchWorkflow = "minekube/actions/.github/workflows/dispatch-workflow.yml@1965ed6ae602a602f9f98edcb31fe177403e8d77"

const mergeToReleasePolicy = `# Merge-to-release policy (authoritative):
# - Every push to master runs Release Please. When it finds a release-eligible
#   change, this workflow merges the generated release PR, reruns itself on
#   master, creates the tag and GitHub release, and calls release-publish.yml.
# - Merging a release-eligible change to master therefore approves that release.
#   PR text, labels, and task-local instructions such as "do not release" do not
#   suppress it. A release hold must first be implemented in this workflow and
#   protected by matching regression coverage.`

var immutableWorkflowRef = regexp.MustCompile(`^[0-9a-f]{40}$`)

func normalizeWorkflowLineEndings(contents string) string {
	return strings.ReplaceAll(contents, "\r\n", "\n")
}

func TestNormalizeWorkflowLineEndings(t *testing.T) {
	const workflow = "name: Release Please\n\n# Merge-to-release policy (authoritative):\n"
	if got := normalizeWorkflowLineEndings(strings.ReplaceAll(workflow, "\n", "\r\n")); got != workflow {
		t.Fatalf("normalized workflow = %q, want %q", got, workflow)
	}
}

func TestReleasePleaseMergeToReleasePolicy(t *testing.T) {
	workflowBytes, err := os.ReadFile(".github/workflows/release-please.yml")
	if err != nil {
		t.Fatal(err)
	}
	contents := normalizeWorkflowLineEndings(string(workflowBytes))
	workflowBytes = []byte(contents)
	if !strings.Contains(contents, mergeToReleasePolicy) {
		t.Fatal("release-please workflow must state the authoritative merge-to-release policy")
	}

	var workflow struct {
		On struct {
			Push struct {
				Branches []string `yaml:"branches"`
			} `yaml:"push"`
			RepositoryDispatch struct {
				Types []string `yaml:"types"`
			} `yaml:"repository_dispatch"`
		} `yaml:"on"`
		Jobs map[string]struct {
			Needs string `yaml:"needs"`
			If    string `yaml:"if"`
			Uses  string `yaml:"uses"`
			Steps []struct {
				Name string `yaml:"name"`
				If   string `yaml:"if"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(workflowBytes, &workflow); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(workflow.On.Push.Branches, []string{"master"}) {
		t.Fatalf("release-please push branches = %v, want [master]", workflow.On.Push.Branches)
	}
	if !reflect.DeepEqual(workflow.On.RepositoryDispatch.Types, []string{"release-please-rerun"}) {
		t.Fatalf("release-please repository dispatch types = %v, want [release-please-rerun]", workflow.On.RepositoryDispatch.Types)
	}

	releasePlease := workflow.Jobs["release-please"]
	if releasePlease.If != "" {
		t.Fatalf("release-please job has an additional gate %q", releasePlease.If)
	}
	var mergeStep *struct {
		Name string `yaml:"name"`
		If   string `yaml:"if"`
		Run  string `yaml:"run"`
	}
	for i := range releasePlease.Steps {
		if releasePlease.Steps[i].Name == "Auto-merge release PR" {
			mergeStep = &releasePlease.Steps[i]
			break
		}
	}
	if mergeStep == nil {
		t.Fatal("release-please must auto-merge its generated release PR")
	}
	if mergeStep.If != "${{ steps.rp.outputs.pr }}" {
		t.Fatalf("release PR merge gate = %q, want only the release-please PR output", mergeStep.If)
	}
	for _, command := range []string{`gh pr merge "$PR_NUMBER"`, "--merge", `-f event_type=release-please-rerun`} {
		if !strings.Contains(mergeStep.Run, command) {
			t.Fatalf("release PR merge step must contain %q", command)
		}
	}

	trigger := workflow.Jobs["trigger-release"]
	if trigger.Needs != "release-please" || trigger.If != "needs.release-please.outputs.release_created == 'true'" {
		t.Fatalf("publisher release gate = needs %q, if %q", trigger.Needs, trigger.If)
	}
	if trigger.Uses != "./.github/workflows/release-publish.yml" {
		t.Fatalf("publisher workflow = %q, want ./.github/workflows/release-publish.yml", trigger.Uses)
	}
}

func TestReleasePleaseMoxyDispatchWorkflowContract(t *testing.T) {
	workflowBytes, err := os.ReadFile(".github/workflows/release-please.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflowBytes = []byte(normalizeWorkflowLineEndings(string(workflowBytes)))

	var workflow struct {
		Jobs map[string]struct {
			Needs       string            `yaml:"needs"`
			If          string            `yaml:"if"`
			Uses        string            `yaml:"uses"`
			Permissions map[string]string `yaml:"permissions"`
			With        map[string]string `yaml:"with"`
			Secrets     string            `yaml:"secrets"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(workflowBytes, &workflow); err != nil {
		t.Fatal(err)
	}

	dispatch, ok := workflow.Jobs["dispatch-moxy-bump"]
	if !ok {
		t.Fatal("dispatch-moxy-bump job is missing")
	}
	if dispatch.Uses != approvedDispatchWorkflow {
		t.Fatalf("dispatch workflow = %q, want %q", dispatch.Uses, approvedDispatchWorkflow)
	}
	if dispatch.Needs != "release-please" || dispatch.If != "needs.release-please.outputs.release_created == 'true'" {
		t.Fatalf("dispatch release gate = needs %q, if %q", dispatch.Needs, dispatch.If)
	}
	if !reflect.DeepEqual(dispatch.Permissions, map[string]string{"contents": "read", "id-token": "write"}) {
		t.Fatalf("dispatch permissions = %#v", dispatch.Permissions)
	}
	if dispatch.Secrets != "inherit" {
		t.Fatalf("dispatch secrets = %q, want inherit", dispatch.Secrets)
	}

	_, ref, ok := strings.Cut(dispatch.Uses, "@")
	if !ok || !immutableWorkflowRef.MatchString(ref) {
		t.Fatalf("dispatch workflow ref = %q, want a 40-character lowercase commit SHA", ref)
	}

	wantInputs := map[string]string{
		"target-repository": "moxy",
		"target-workflow":   "bump-gate.yml",
		"target-ref":        "main",
		"inputs-json": `{
  "version": "${{ needs.release-please.outputs.tag_name }}",
  "source_repository": "${{ github.repository }}",
  "source_release_url": "${{ github.server_url }}/${{ github.repository }}/releases/tag/${{ needs.release-please.outputs.tag_name }}"
}
`,
	}
	if !reflect.DeepEqual(dispatch.With, wantInputs) {
		t.Fatalf("dispatch inputs = %#v, want %#v", dispatch.With, wantInputs)
	}
}
