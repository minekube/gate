package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// .web (the VitePress site + Cloudflare Worker that serves gate.minekube.com)
// carries its own contract suite and build. This policy test deliberately
// validates the complete security-relevant workflow structure: accepting one
// reviewed fixture is insufficient if an inert command, extra trigger, broader
// permission, mutable action, or ambiguous YAML shape can survive.

const webWorkflowPath = ".github/workflows/web.yml"

const (
	webInstallCommand = "pnpm install --frozen-lockfile"
	webTestCommand    = "node --test scripts/*.test.mjs"
	webBuildCommand   = "pnpm run build"

	checkoutAction = "actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803"   // v6
	pnpmAction     = "pnpm/action-setup@0977fd99725f1db4007ccb2928dbb4e90d06cc86"  // v6
	nodeAction     = "actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38" // v6
)

func TestWebWorkflowRunsTheDotWebChecks(t *testing.T) {
	assertWebWorkflowPolicy(t)
}

func TestWebWorkflowIsPathFilteredToDotWeb(t *testing.T) {
	assertWebWorkflowPolicy(t)
}

func TestWebWorkflowIsReadOnlyAndSecretFree(t *testing.T) {
	assertWebWorkflowPolicy(t)
}

func assertWebWorkflowPolicy(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(webWorkflowPath)
	if err != nil {
		t.Fatalf("web workflow policy: %s is missing: %v", webWorkflowPath, err)
	}
	if err := validateWebWorkflow(raw); err != nil {
		t.Fatalf("web workflow policy: %v", err)
	}
}

func validateWebWorkflow(raw []byte) error {
	root, err := parseWebWorkflow(raw)
	if err != nil {
		return err
	}

	top, err := exactMapping(root, "workflow", "name", "on", "permissions", "concurrency", "jobs")
	if err != nil {
		return err
	}
	if err := exactScalar(top["name"], "workflow.name", "web"); err != nil {
		return err
	}

	triggers, err := exactMapping(top["on"], "workflow.on", "push", "pull_request")
	if err != nil {
		return err
	}
	push, err := exactMapping(triggers["push"], "workflow.on.push", "branches", "paths")
	if err != nil {
		return err
	}
	if err := exactStringSequence(push["branches"], "workflow.on.push.branches", "master"); err != nil {
		return err
	}
	if err := exactStringSequence(push["paths"], "workflow.on.push.paths", ".web/**", webWorkflowPath); err != nil {
		return err
	}
	pullRequest, err := exactMapping(triggers["pull_request"], "workflow.on.pull_request", "paths")
	if err != nil {
		return err
	}
	if err := exactStringSequence(pullRequest["paths"], "workflow.on.pull_request.paths", ".web/**", webWorkflowPath); err != nil {
		return err
	}

	if _, err := exactMapping(top["permissions"], "workflow.permissions"); err != nil {
		return err
	}
	concurrency, err := exactMapping(top["concurrency"], "workflow.concurrency", "group", "cancel-in-progress")
	if err != nil {
		return err
	}
	if err := exactScalar(concurrency["group"], "workflow.concurrency.group", "web-${{ github.ref }}"); err != nil {
		return err
	}
	if err := exactBool(concurrency["cancel-in-progress"], "workflow.concurrency.cancel-in-progress", true); err != nil {
		return err
	}

	jobs, err := exactMapping(top["jobs"], "workflow.jobs", "web")
	if err != nil {
		return err
	}
	job, err := exactMapping(jobs["web"], "workflow.jobs.web", "name", "runs-on", "permissions", "defaults", "steps")
	if err != nil {
		return err
	}
	if err := exactScalar(job["name"], "workflow.jobs.web.name", "docs build + contract tests"); err != nil {
		return err
	}
	if err := exactScalar(job["runs-on"], "workflow.jobs.web.runs-on", "ubuntu-latest"); err != nil {
		return err
	}
	permissions, err := exactMapping(job["permissions"], "workflow.jobs.web.permissions", "contents")
	if err != nil {
		return err
	}
	if err := exactScalar(permissions["contents"], "workflow.jobs.web.permissions.contents", "read"); err != nil {
		return err
	}
	defaults, err := exactMapping(job["defaults"], "workflow.jobs.web.defaults", "run")
	if err != nil {
		return err
	}
	runDefaults, err := exactMapping(defaults["run"], "workflow.jobs.web.defaults.run", "working-directory")
	if err != nil {
		return err
	}
	if err := exactScalar(runDefaults["working-directory"], "workflow.jobs.web.defaults.run.working-directory", ".web"); err != nil {
		return err
	}

	return validateWebSteps(job["steps"])
}

func validateWebSteps(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.SequenceNode {
		return fmt.Errorf("workflow.jobs.web.steps must be a sequence")
	}
	if len(node.Content) != 6 {
		return fmt.Errorf("workflow.jobs.web.steps has %d entries, want exactly 6", len(node.Content))
	}

	checkout, err := exactMapping(node.Content[0], "workflow.jobs.web.steps[0]", "name", "uses", "with")
	if err != nil {
		return err
	}
	if err := exactScalar(checkout["name"], "workflow.jobs.web.steps[0].name", "Checkout"); err != nil {
		return err
	}
	if err := exactScalar(checkout["uses"], "workflow.jobs.web.steps[0].uses", checkoutAction); err != nil {
		return err
	}
	checkoutWith, err := exactMapping(checkout["with"], "workflow.jobs.web.steps[0].with", "persist-credentials")
	if err != nil {
		return err
	}
	if err := exactBool(checkoutWith["persist-credentials"], "workflow.jobs.web.steps[0].with.persist-credentials", false); err != nil {
		return err
	}

	pnpm, err := exactMapping(node.Content[1], "workflow.jobs.web.steps[1]", "name", "uses", "with")
	if err != nil {
		return err
	}
	if err := exactScalar(pnpm["name"], "workflow.jobs.web.steps[1].name", "Set up pnpm"); err != nil {
		return err
	}
	if err := exactScalar(pnpm["uses"], "workflow.jobs.web.steps[1].uses", pnpmAction); err != nil {
		return err
	}
	pnpmWith, err := exactMapping(pnpm["with"], "workflow.jobs.web.steps[1].with", "package_json_file", "cache", "cache_dependency_path")
	if err != nil {
		return err
	}
	if err := exactScalar(pnpmWith["package_json_file"], "workflow.jobs.web.steps[1].with.package_json_file", ".web/package.json"); err != nil {
		return err
	}
	if err := exactBool(pnpmWith["cache"], "workflow.jobs.web.steps[1].with.cache", true); err != nil {
		return err
	}
	if err := exactScalar(pnpmWith["cache_dependency_path"], "workflow.jobs.web.steps[1].with.cache_dependency_path", ".web/pnpm-lock.yaml"); err != nil {
		return err
	}

	nodeSetup, err := exactMapping(node.Content[2], "workflow.jobs.web.steps[2]", "name", "uses", "with")
	if err != nil {
		return err
	}
	if err := exactScalar(nodeSetup["name"], "workflow.jobs.web.steps[2].name", "Set up Node"); err != nil {
		return err
	}
	if err := exactScalar(nodeSetup["uses"], "workflow.jobs.web.steps[2].uses", nodeAction); err != nil {
		return err
	}
	nodeWith, err := exactMapping(nodeSetup["with"], "workflow.jobs.web.steps[2].with", "node-version")
	if err != nil {
		return err
	}
	if err := exactScalar(nodeWith["node-version"], "workflow.jobs.web.steps[2].with.node-version", "22"); err != nil {
		return err
	}

	for i, want := range []struct {
		name    string
		command string
	}{
		{"Install dependencies", webInstallCommand},
		{"Contract tests", webTestCommand},
		{"Build docs site", webBuildCommand},
	} {
		stepIndex := i + 3
		context := fmt.Sprintf("workflow.jobs.web.steps[%d]", stepIndex)
		step, err := exactMapping(node.Content[stepIndex], context, "name", "run")
		if err != nil {
			return err
		}
		if err := exactScalar(step["name"], context+".name", want.name); err != nil {
			return err
		}
		if err := exactScalar(step["run"], context+".run", want.command); err != nil {
			return err
		}
	}
	return nil
}

func parseWebWorkflow(raw []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("parse %s: %w", webWorkflowPath, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%s contains more than one YAML document", webWorkflowPath)
		}
		return nil, fmt.Errorf("parse trailing YAML in %s: %w", webWorkflowPath, err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, fmt.Errorf("%s must contain one YAML document", webWorkflowPath)
	}
	if err := validateYAMLIntegrity(document.Content[0], "workflow"); err != nil {
		return nil, err
	}
	return document.Content[0], nil
}

func validateYAMLIntegrity(node *yaml.Node, context string) error {
	if node == nil {
		return fmt.Errorf("%s is missing", context)
	}
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("%s uses a YAML alias; aliases are not allowed in the protected workflow", context)
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%2 != 0 {
			return fmt.Errorf("%s has a malformed mapping", context)
		}
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("%s has a non-scalar mapping key", context)
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return fmt.Errorf("%s contains duplicate key %q", context, key.Value)
			}
			seen[key.Value] = struct{}{}
			if err := validateYAMLIntegrity(node.Content[i+1], context+"."+key.Value); err != nil {
				return err
			}
		}
		return nil
	}
	for i, child := range node.Content {
		if err := validateYAMLIntegrity(child, fmt.Sprintf("%s[%d]", context, i)); err != nil {
			return err
		}
	}
	return nil
}

func exactMapping(node *yaml.Node, context string, allowed ...string) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", context)
	}
	values := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		values[node.Content[i].Value] = node.Content[i+1]
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
		if _, ok := values[key]; !ok {
			return nil, fmt.Errorf("%s is missing required key %q", context, key)
		}
	}
	for key := range values {
		if _, ok := allowedSet[key]; !ok {
			return nil, fmt.Errorf("%s contains unexpected key %q", context, key)
		}
	}
	return values, nil
}

func exactStringSequence(node *yaml.Node, context string, want ...string) error {
	if node == nil || node.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s must be a sequence", context)
	}
	if len(node.Content) != len(want) {
		return fmt.Errorf("%s has %d entries, want exactly %d (%v)", context, len(node.Content), len(want), want)
	}
	for i, expected := range want {
		if err := exactScalar(node.Content[i], fmt.Sprintf("%s[%d]", context, i), expected); err != nil {
			return err
		}
	}
	return nil
}

func exactScalar(node *yaml.Node, context, want string) error {
	if node == nil || node.Kind != yaml.ScalarNode {
		return fmt.Errorf("%s must be scalar %q", context, want)
	}
	if node.Value != want {
		return fmt.Errorf("%s = %q, want exactly %q", context, node.Value, want)
	}
	return nil
}

func exactBool(node *yaml.Node, context string, want bool) error {
	value := "false"
	if want {
		value = "true"
	}
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!bool" || node.Value != value {
		return fmt.Errorf("%s must be boolean %s", context, value)
	}
	return nil
}
