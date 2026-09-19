package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// .web (the VitePress site + Cloudflare Worker that serves gate.minekube.com)
// carries its own checks - `node --test scripts/*.test.mjs` (the Worker and
// docs-deployment contract suite) and `pnpm run build` - and
// .github/workflows/web.yml is the ONLY thing that runs them.
//
// The failure mode this pins down is silent: before web.yml existed the
// contract suite sat red on master across several Renovate bumps, because no
// workflow executed it (minekube/gate#1144 - the test still pinned
// pnpm@10.11.0/wrangler 4.115.0 after package.json had moved on). Deleting
// the job, narrowing its path filter, or dropping a step would recreate
// exactly that blind spot, and nothing else in CI would notice.
//
// The checks below are therefore fail-closed on the properties that keep the
// suite wired, cheap and read-only.

const webWorkflowPath = ".github/workflows/web.yml"

// Commands the .web checks are defined by (package.json "test" + "build").
const (
	webInstallCommand = "pnpm install --frozen-lockfile"
	webTestCommand    = "node --test scripts/*.test.mjs"
	webBuildCommand   = "pnpm run build"
)

type webWorkflow struct {
	On          webWorkflowTriggers       `yaml:"on"`
	Permissions map[string]string         `yaml:"permissions"`
	Jobs        map[string]webWorkflowJob `yaml:"jobs"`
}

type webWorkflowTriggers struct {
	Push             *webWorkflowPathFilter `yaml:"push"`
	PullRequest      *webWorkflowPathFilter `yaml:"pull_request"`
	WorkflowDispatch *yaml.Node             `yaml:"workflow_dispatch"`
}

type webWorkflowPathFilter struct {
	Paths    []string `yaml:"paths"`
	Branches []string `yaml:"branches"`
	// Types defaults to [opened, synchronize, reopened]. A workflow that
	// narrows it (as ci.yml does) never re-runs on the push that fixes a red
	// docs check.
	Types []string `yaml:"types"`
}

type webWorkflowJob struct {
	Permissions map[string]string `yaml:"permissions"`
	Defaults    struct {
		Run struct {
			WorkingDirectory string `yaml:"working-directory"`
		} `yaml:"run"`
	} `yaml:"defaults"`
	Steps []webWorkflowStep `yaml:"steps"`
}

type webWorkflowStep struct {
	Name string         `yaml:"name"`
	Uses string         `yaml:"uses"`
	Run  string         `yaml:"run"`
	With map[string]any `yaml:"with"`
}

func readWebWorkflow(t *testing.T) (webWorkflow, string) {
	t.Helper()

	raw, err := os.ReadFile(webWorkflowPath)
	if err != nil {
		t.Fatalf("%s is missing (%v): nothing else in .github/workflows runs the .web contract tests or the docs build, so "+
			"they go red on master unnoticed - see minekube/gate#1144", webWorkflowPath, err)
	}

	var workflow webWorkflow
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("parse %s: %v", webWorkflowPath, err)
	}
	if len(workflow.Jobs) == 0 {
		t.Fatalf("%s defines no jobs", webWorkflowPath)
	}

	return workflow, string(raw)
}

// stepIndexes maps each required command to the index of the step running it.
func stepIndexes(t *testing.T, jobID string, job webWorkflowJob) map[string]int {
	t.Helper()

	indexes := make(map[string]int, 3)
	for i, step := range job.Steps {
		for _, command := range []string{webInstallCommand, webTestCommand, webBuildCommand} {
			if !strings.Contains(step.Run, command) {
				continue
			}
			if previous, ok := indexes[command]; ok {
				t.Errorf("%s: job %q runs %q in two steps (%d, %d); the check must have a single source of truth",
					webWorkflowPath, jobID, command, previous, i)
				continue
			}
			indexes[command] = i
		}
	}

	return indexes
}

func TestWebWorkflowRunsTheDotWebChecks(t *testing.T) {
	workflow, _ := readWebWorkflow(t)

	jobsCoveringWeb := 0
	for jobID, job := range workflow.Jobs {
		indexes := stepIndexes(t, jobID, job)
		if _, ok := indexes[webInstallCommand]; !ok {
			continue // job does not cover .web, so it is not responsible for these checks
		}
		jobsCoveringWeb++

		for _, command := range []string{webTestCommand, webBuildCommand} {
			if _, ok := indexes[command]; !ok {
				t.Errorf("%s: job %q installs .web dependencies but never runs %q, so the check cannot fail CI",
					webWorkflowPath, jobID, command)
			}
		}
		if t.Failed() {
			continue
		}

		if indexes[webInstallCommand] > indexes[webTestCommand] || indexes[webTestCommand] > indexes[webBuildCommand] {
			t.Errorf("%s: job %q must install (%d) then test (%d) then build (%d)",
				webWorkflowPath, jobID, indexes[webInstallCommand], indexes[webTestCommand], indexes[webBuildCommand])
		}

		// The commands are repo-root relative only because the job either sets
		// a working directory or carries the directory in the command itself.
		if job.Defaults.Run.WorkingDirectory != ".web" {
			for _, command := range []string{webInstallCommand, webTestCommand, webBuildCommand} {
				if !strings.Contains(job.Steps[indexes[command]].Run, ".web") {
					t.Errorf("%s: job %q runs %q (step %d) without working-directory .web or a .web path",
						webWorkflowPath, jobID, command, indexes[command])
				}
			}
		}

		// pnpm must be installed before anything uses it, and the Node the docs
		// build needs (engines: >=22) must be pinned explicitly.
		checkout, pnpm, node := -1, -1, -1
		for i, step := range job.Steps {
			switch {
			case strings.HasPrefix(step.Uses, "actions/checkout@"):
				checkout = i
			case strings.HasPrefix(step.Uses, "pnpm/action-setup@"):
				pnpm = i
			case strings.HasPrefix(step.Uses, "actions/setup-node@"):
				node = i
			}
		}
		switch {
		case checkout < 0:
			t.Errorf("%s: job %q never checks out the repository", webWorkflowPath, jobID)
		case pnpm < 0:
			// A bare `pnpm` is spawned by scripts/build-with-community-stats.mjs
			// and corepack is not bundled on the runners' Node 25+, so pnpm has
			// to be installed explicitly.
			t.Errorf("%s: job %q never installs pnpm, but `pnpm run build` and the build script's bare `pnpm` need it on PATH",
				webWorkflowPath, jobID)
		case checkout > pnpm || pnpm > indexes[webInstallCommand]:
			t.Errorf("%s: job %q must checkout (%d) and set up pnpm (%d) before installing (%d)",
				webWorkflowPath, jobID, checkout, pnpm, indexes[webInstallCommand])
		}
		if node < 0 || node > indexes[webInstallCommand] {
			t.Errorf("%s: job %q must set up Node before installing (setup-node at %d, install at %d)",
				webWorkflowPath, jobID, node, indexes[webInstallCommand])
		} else {
			version := fmt.Sprint(job.Steps[node].With["node-version"])
			if major, err := strconv.Atoi(strings.SplitN(version, ".", 2)[0]); err != nil || major < 22 {
				t.Errorf("%s: job %q pins node-version %q; .web/package.json requires >=22.0.0",
					webWorkflowPath, jobID, version)
			}
		}

		// Deriving the pnpm version from `.web/package.json`'s packageManager
		// keeps CI on the toolchain the docs are built and deployed with.
		if pnpm >= 0 {
			with := job.Steps[pnpm].With
			fromPackageJSON := fmt.Sprint(with["package_json_file"]) == ".web/package.json"
			version, pinned := with["version"]
			if !fromPackageJSON && (!pinned || fmt.Sprint(version) == "latest") {
				t.Errorf("%s: job %q does not pin the pnpm version: set package_json_file: .web/package.json or an explicit version",
					webWorkflowPath, jobID)
			}
		}
	}

	if jobsCoveringWeb == 0 {
		t.Fatalf("%s: no job runs %q, so nothing fails CI when the .web checks break", webWorkflowPath, webInstallCommand)
	}
}

func TestWebWorkflowIsPathFilteredToDotWeb(t *testing.T) {
	workflow, _ := readWebWorkflow(t)

	for name, trigger := range map[string]*webWorkflowPathFilter{
		"push":         workflow.On.Push,
		"pull_request": workflow.On.PullRequest,
	} {
		if trigger == nil {
			t.Errorf("%s: %s trigger is absent; the .web checks must run on %s to .web/** (and on master pushes)",
				webWorkflowPath, name, name)
			continue
		}
		if len(trigger.Paths) == 0 {
			t.Errorf("%s: %s runs on every change; filter it to .web/** so unrelated PRs do not pay for a docs job",
				webWorkflowPath, name)
		}
		for _, path := range trigger.Paths {
			switch strings.TrimSpace(path) {
			case "", "*", "**", "**/*", ".github/**":
				t.Errorf("%s: %s path filter %q is a catch-all, which defeats the point of filtering the docs checks",
					webWorkflowPath, name, path)
			}
		}
		if !containsString(trigger.Paths, ".web/**") {
			t.Errorf("%s: %s paths = %v, want it to include .web/**", webWorkflowPath, name, trigger.Paths)
		}
		if !containsString(trigger.Paths, webWorkflowPath) {
			t.Errorf("%s: %s paths = %v, want it to include %s so editing the workflow itself runs it",
				webWorkflowPath, name, trigger.Paths, webWorkflowPath)
		}
	}

	// PRs already run on open/synchronize/reopen. Restricting push to master
	// preserves a post-merge/direct-push gate without scheduling the same
	// workflow twice for every feature-branch commit.
	if branches := workflow.On.Push.Branches; len(branches) != 1 || branches[0] != "master" {
		t.Errorf("%s: push branches = %v, want [master]; unrestricted push duplicates the pull_request run on every feature-branch commit",
			webWorkflowPath, branches)
	}

	// Unlike ci.yml's [opened, reopened] filter, a fix pushed to the same PR
	// must re-run these checks.
	if types := workflow.On.PullRequest.Types; len(types) > 0 {
		if !containsString(types, "synchronize") {
			t.Errorf("%s: pull_request types = %v; without synchronize a pushed fix never re-runs the .web checks",
				webWorkflowPath, types)
		}
	}
}

func TestWebWorkflowIsReadOnlyAndSecretFree(t *testing.T) {
	workflow, raw := readWebWorkflow(t)

	if len(workflow.Permissions) != 0 {
		t.Errorf("%s: workflow-level permissions = %v, want empty (grant per job)", webWorkflowPath, workflow.Permissions)
	}
	for jobID, job := range workflow.Jobs {
		for scope, access := range job.Permissions {
			if scope != "contents" || access != "read" {
				t.Errorf("%s: job %q grants %s: %s; the docs checks only need contents: read",
					webWorkflowPath, jobID, scope, access)
			}
		}
		if len(job.Permissions) == 0 {
			t.Errorf("%s: job %q declares no permissions; it must not inherit the workflow's empty set implicitly",
				webWorkflowPath, jobID)
		}
		for i, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/checkout@") {
				if persist, ok := step.With["persist-credentials"]; !ok || fmt.Sprint(persist) != "false" {
					t.Errorf("%s: job %q checkout step %d does not set persist-credentials: false",
						webWorkflowPath, jobID, i)
				}
			}
		}
	}

	// The jobs run .web sources (checked out from a PR), so no credential may
	// be reachable from them.
	if strings.Contains(raw, "secrets.") {
		t.Errorf("%s: references a secret; the docs checks run pull-request-controlled sources and must hold no credential",
			webWorkflowPath)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
