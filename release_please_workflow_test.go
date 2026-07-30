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

var immutableWorkflowRef = regexp.MustCompile(`^[0-9a-f]{40}$`)

func TestReleasePleaseMoxyDispatchWorkflowContract(t *testing.T) {
	workflowBytes, err := os.ReadFile(".github/workflows/release-please.yml")
	if err != nil {
		t.Fatal(err)
	}

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
