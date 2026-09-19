package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// These are deliberate one-property weakenings of the protected web workflow.
// Each candidate must remain YAML-parseable and must be rejected by the real
// repository policy tests for a "web workflow policy" reason.
func TestWebWorkflowRejectsWeakeningMutations(t *testing.T) {
	if os.Getenv("GATE_WEB_WORKFLOW_MUTATION_CHILD") == "1" {
		t.Skip("mutation child runs only the workflow policy tests")
	}

	baseline, err := os.ReadFile(webWorkflowPath)
	if err != nil {
		t.Fatal(err)
	}

	mutations := []webWorkflowMutation{
		replaceMutation("drop-install", []string{"run: pnpm install --frozen-lockfile"}, "run: pnpm install"),
		replaceMutation("drop-contract-test", []string{"run: node --test scripts/*.test.mjs"}, "run: true # contract test removed"),
		replaceMutation("drop-build", []string{"run: pnpm run build"}, "run: true # build removed"),
		replaceMutation("install-echo-inert", []string{"run: pnpm install --frozen-lockfile"}, "run: echo 'pnpm install --frozen-lockfile'"),
		replaceMutation("test-echo-inert", []string{"run: node --test scripts/*.test.mjs"}, "run: echo 'node --test scripts/*.test.mjs'"),
		replaceMutation("build-echo-inert", []string{"run: pnpm run build"}, "run: echo 'pnpm run build'"),
		replaceMutation("install-comment-decoy", []string{"run: pnpm install --frozen-lockfile"}, "run: |\n          # pnpm install --frozen-lockfile\n          true"),
		replaceMutation("test-comment-decoy", []string{"run: node --test scripts/*.test.mjs"}, "run: |\n          # node --test scripts/*.test.mjs\n          true"),
		replaceMutation("build-comment-decoy", []string{"run: pnpm run build"}, "run: |\n          # pnpm run build\n          true"),
		replaceMutation("install-dead-shell-branch", []string{"run: pnpm install --frozen-lockfile"}, "run: if false; then pnpm install --frozen-lockfile; fi"),
		replaceMutation("install-or-true", []string{"run: pnpm install --frozen-lockfile"}, "run: pnpm install --frozen-lockfile || true"),
		replaceMutation("test-or-true", []string{"run: node --test scripts/*.test.mjs"}, "run: node --test scripts/*.test.mjs || true"),
		replaceMutation("build-or-true", []string{"run: pnpm run build"}, "run: pnpm run build || true"),
		replaceMutation("job-unreachable-if", []string{"  web:\n    name: docs build + contract tests"}, "  web:\n    name: docs build + contract tests\n    if: github.repository == 'attacker/not-gate'"),
		replaceMutation("contract-step-unreachable-if", []string{"      - name: Contract tests\n        run: node --test scripts/*.test.mjs"}, "      - name: Contract tests\n        if: github.repository == 'attacker/not-gate'\n        run: node --test scripts/*.test.mjs"),
		replaceMutation("build-continue-on-error", []string{"      - name: Build docs site\n        run: pnpm run build"}, "      - name: Build docs site\n        continue-on-error: true\n        run: pnpm run build"),
		replaceMutation("contract-step-root-working-directory", []string{"      - name: Contract tests\n        run: node --test scripts/*.test.mjs"}, "      - name: Contract tests\n        working-directory: .\n        run: node --test scripts/*.test.mjs"),
		replaceMutation("duplicate-contract-command", []string{"      - name: Contract tests\n        run: node --test scripts/*.test.mjs"}, "      - name: Contract tests\n        run: node --test scripts/*.test.mjs\n      - name: Decoy contract tests\n        run: node --test scripts/*.test.mjs"),
		replaceMutation("build-before-test", []string{"      - name: Contract tests\n        run: node --test scripts/*.test.mjs\n\n      # scripts/build-with-community-stats.mjs spawns a bare `pnpm`, so pnpm\n      # has to be on PATH (the step above, not npx) - and Node >= 22.\n      - name: Build docs site\n        run: pnpm run build"}, "      - name: Build docs site\n        run: pnpm run build\n\n      - name: Contract tests\n        run: node --test scripts/*.test.mjs"),
		replaceMutation("wrong-job-working-directory", []string{"working-directory: .web"}, "working-directory: ."),
		replaceMutation("extra-unrelated-step", []string{"      - name: Install dependencies\n        run: pnpm install --frozen-lockfile"}, "      - name: Unrelated command\n        run: echo unrelated\n      - name: Install dependencies\n        run: pnpm install --frozen-lockfile"),

		replaceMutation("wrong-pnpm-action", []string{"uses: pnpm/action-setup@v6", "uses: pnpm/action-setup@0977fd99725f1db4007ccb2928dbb4e90d06cc86"}, "uses: owner/not-pnpm-action@0977fd99725f1db4007ccb2928dbb4e90d06cc86"),
		replaceMutation("floating-pnpm-action", []string{"uses: pnpm/action-setup@v6", "uses: pnpm/action-setup@0977fd99725f1db4007ccb2928dbb4e90d06cc86"}, "uses: pnpm/action-setup@main"),
		replaceMutation("floating-checkout-action", []string{"uses: actions/checkout@v6", "uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803"}, "uses: actions/checkout@main"),
		replaceMutation("floating-node-action", []string{"uses: actions/setup-node@v6", "uses: actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38"}, "uses: actions/setup-node@main"),
		replaceMutation("wrong-checkout-action", []string{"uses: actions/checkout@v6", "uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803"}, "uses: attacker/checkout@d23441a48e516b6c34aea4fa41551a30e30af803"),
		replaceMutation("wrong-node-action", []string{"uses: actions/setup-node@v6", "uses: actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38"}, "uses: attacker/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38"),
		replaceMutation("node21", []string{"node-version: 22"}, "node-version: 21"),
		replaceMutation("unbound-pnpm-version", []string{"package_json_file: .web/package.json"}, "package_json_file: package.json"),
		replaceMutation("pnpm-explicit-latest-overrides-package", []string{"          package_json_file: .web/package.json\n          cache: true"}, "          package_json_file: .web/package.json\n          version: latest\n          cache: true"),

		insertBeforeMutation("extra-pull-request-target-trigger", "\n# Empty at workflow level;", "\n  pull_request_target:\n    paths:\n      - .web/**\n      - .github/workflows/web.yml\n"),
		insertBeforeMutation("extra-schedule-trigger", "\n# Empty at workflow level;", "\n  schedule:\n    - cron: '0 * * * *'\n"),
		insertBeforeMutation("extra-workflow-dispatch-trigger", "\n# Empty at workflow level;", "\n  workflow_dispatch:\n"),
		replaceMutation("extra-unrelated-pr-path", []string{"  pull_request:\n    paths:\n      - .web/**\n      - .github/workflows/web.yml"}, "  pull_request:\n    paths:\n      - .web/**\n      - .github/workflows/web.yml\n      - pkg/**"),
		replaceMutation("negate-dotweb-pr-path", []string{"  pull_request:\n    paths:\n      - .web/**\n      - .github/workflows/web.yml"}, "  pull_request:\n    paths:\n      - .web/**\n      - .github/workflows/web.yml\n      - '!.web/**'"),
		replaceMutation("remove-web-path", []string{"  pull_request:\n    paths:\n      - .web/**\n      - .github/workflows/web.yml"}, "  pull_request:\n    paths:\n      - docs/**\n      - .github/workflows/web.yml"),
		replaceMutation("ignore-master-pr-base", []string{"  pull_request:\n    paths:"}, "  pull_request:\n    branches-ignore: [master]\n    paths:"),
		replaceMutation("extra-closed-pr-type", []string{"  pull_request:\n    paths:"}, "  pull_request:\n    types: [opened, synchronize, reopened, closed]\n    paths:"),
		replaceMutation("drop-pr-synchronize", []string{"  pull_request:\n    paths:"}, "  pull_request:\n    types: [opened, reopened]\n    paths:"),
		replaceMutation("unrestricted-push", []string{"  push:\n    branches:\n      - master\n    paths:"}, "  push:\n    paths:"),
		replaceMutation("extra-push-branch", []string{"    branches:\n      - master\n    paths:"}, "    branches:\n      - master\n      - develop\n    paths:"),

		replaceMutation("write-job-permission", []string{"      contents: read\n    defaults:"}, "      contents: write\n    defaults:"),
		replaceMutation("persist-checkout-token", []string{"persist-credentials: false"}, "persist-credentials: true"),
		replaceMutation("workflow-write-permission", []string{"permissions: {}"}, "permissions:\n  contents: write"),
		replaceMutation("checkout-bracket-secret-token", []string{"          persist-credentials: false"}, "          persist-credentials: false\n          token: ${{ secrets['DEPLOY_TOKEN'] }}"),
		insertBeforeMutation("workflow-bracket-secret-env", "\nconcurrency:", "\nenv:\n  LEAK: ${{ secrets['DEPLOY_TOKEN'] }}\n"),
		replaceMutation("step-dot-secret-reference", []string{"        run: pnpm run build"}, "        env:\n          EXAMPLE: ${{ secrets.EXAMPLE }}\n        run: pnpm run build"),
		insertBeforeMutation("extra-job", "\n  web:\n", "\n  unrelated:\n    runs-on: ubuntu-latest\n    steps:\n      - run: true\n"),
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	for _, mutation := range mutations {
		mutation := mutation
		t.Run(mutation.id, func(t *testing.T) {
			candidate, err := mutation.apply(baseline)
			if err != nil {
				t.Fatal(err)
			}
			var parsed yaml.Node
			if err := yaml.Unmarshal(candidate, &parsed); err != nil {
				t.Fatalf("mutation is not YAML-parseable: %v", err)
			}

			tmp := t.TempDir()
			workflowDir := filepath.Join(tmp, ".github", "workflows")
			if err := os.MkdirAll(workflowDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workflowDir, "web.yml"), candidate, 0o644); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(executable, "-test.run=^TestWebWorkflow(RunsTheDotWebChecks|IsPathFilteredToDotWeb|IsReadOnlyAndSecretFree)$", "-test.count=1")
			cmd.Dir = tmp
			cmd.Env = append(os.Environ(), "GATE_WEB_WORKFLOW_MUTATION_CHILD=1")
			output, runErr := cmd.CombinedOutput()
			if runErr == nil {
				t.Fatalf("weakening survived policy tests\n%s", output)
			}
			if !bytes.Contains(output, []byte("web workflow policy:")) {
				t.Fatalf("weakening failed for the wrong reason; want a policy assertion\n%s", output)
			}
		})
	}
}

type webWorkflowMutation struct {
	id    string
	apply func([]byte) ([]byte, error)
}

func replaceMutation(id string, alternatives []string, replacement string) webWorkflowMutation {
	return webWorkflowMutation{id: id, apply: func(source []byte) ([]byte, error) {
		matches := 0
		var old string
		for _, alternative := range alternatives {
			count := bytes.Count(source, []byte(alternative))
			matches += count
			if count == 1 {
				old = alternative
			}
		}
		if matches != 1 {
			return nil, fmt.Errorf("mutation %s: replacement matched %d times, want exactly once", id, matches)
		}
		return bytes.Replace(source, []byte(old), []byte(replacement), 1), nil
	}}
}

func insertBeforeMutation(id, anchor, insertion string) webWorkflowMutation {
	return webWorkflowMutation{id: id, apply: func(source []byte) ([]byte, error) {
		if count := strings.Count(string(source), anchor); count != 1 {
			return nil, fmt.Errorf("mutation %s: insertion anchor matched %d times, want exactly once", id, count)
		}
		return bytes.Replace(source, []byte(anchor), []byte(insertion+anchor), 1), nil
	}}
}
