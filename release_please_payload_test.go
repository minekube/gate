package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The "Auto-merge release PR" step used to build its command out of the release
// PR payload with a single-quoted shell literal:
//
//	PR_NUMBER=$(echo '${{ steps.rp.outputs.pr }}' | jq -r '.number')
//
// `steps.rp.outputs.pr` is GitHub's JSON for the generated release PR *including
// its body*, and that body repeats the merged commit subjects verbatim. On
// 2026-09-27 the merge of #1201 (`fix(gate): run Gate's event manager on a
// race-free wrapper`) handed bash this script text:
//
//	PR_NUMBER=$(echo '{"...","body":"...* **gate:** run Gate's event manager ..."}' | jq -r '.number')
//
// The apostrophe in the merged subject closed the quoted literal, so the rest of
// the payload was parsed as shell code and the step died with
//
//	/home/runner/work/_temp/<id>.sh: line 1: syntax error near unexpected token `('
//	##[error]Process completed with exit code 2
//
// (run 36320821502, job 108624180274). `trigger-release` and `dispatch-moxy-bump`
// were skipped, the release PR #1202 stayed `autorelease: pending`, and v0.74.21
// only shipped after the release PR was merged by hand. Authors saw nothing:
// ci.yml was green on the same merge, only "Release Please" failed.
//
// The payload must always reach the shell as DATA (an environment variable),
// never as script text. The tests below pin that boundary, run the step for real
// against the payload that broke it, and prove the detection rejects the
// historical shape.
const (
	releasePleaseWorkflowPath  = ".github/workflows/release-please.yml"
	releasePleaseMergeStepName = "Auto-merge release PR"

	// releasePleasePayloadEnvVar carries `steps.rp.outputs.pr` into the shell as
	// data instead of into the script as code.
	releasePleasePayloadEnvVar = "RP_PR"

	// releasePleasePolicyPrefix marks every assertion below so the mutation
	// harness can tell a policy rejection from an unrelated failure.
	releasePleasePolicyPrefix = "release-please workflow policy:"

	releasePleaseMutationChildEnv = "GATE_RELEASE_PLEASE_PAYLOAD_MUTATION_CHILD"
)

// releasePleasePRBodyApostrophe is the release-notes body from job 108624180274's
// log, verbatim (line breaks included).
const releasePleasePRBodyApostrophe = `:robot: I have created a release *beep* *boop*
---


## [0.74.21](https://github.com/minekube/gate/compare/v0.74.20...v0.74.21) (2026-09-27)


### Bug Fixes

* **gate:** run Gate's event manager on a race-free wrapper ([#1201](https://github.com/minekube/gate/issues/1201)) ([ea61218](https://github.com/minekube/gate/commit/ea6121888ab872780d8445ba46a5441e6a262579))

---
This PR was generated with [Release Please](https://github.com/googleapis/release-please). See [documentation](https://github.com/googleapis/release-please#release-please).`

// releasePleasePRBodyQuoteFree has the same shape as the body above, with a
// merged subject that contains no apostrophe: the release notes that always
// worked, which is why the defect stayed invisible until a subject like
// `Gate's` was released.
const releasePleasePRBodyQuoteFree = `:robot: I have created a release *beep* *boop*
---


## [0.74.22](https://github.com/minekube/gate/compare/v0.74.21...v0.74.22) (2026-09-28)


### Bug Fixes

* **timeouts:** stop scaling the configured read timeouts a second time ([#1210](https://github.com/minekube/gate/issues/1210)) ([ffa58c5](https://github.com/minekube/gate/commit/ffa58c5))

---
This PR was generated with [Release Please](https://github.com/googleapis/release-please). See [documentation](https://github.com/googleapis/release-please#release-please).`

// releasePleasePayload renders the JSON GitHub passes to the step, with the same
// keys and key order as the payload recorded in the failing job's log.
func releasePleasePayload(t *testing.T, number int, title, body string) string {
	t.Helper()
	payload := struct {
		HeadBranchName string   `json:"headBranchName"`
		BaseBranchName string   `json:"baseBranchName"`
		Number         int      `json:"number"`
		Title          string   `json:"title"`
		Body           string   `json:"body"`
		Files          []string `json:"files"`
		Labels         []string `json:"labels"`
	}{
		HeadBranchName: "release-please--branches--master--components--gate",
		BaseBranchName: "master",
		Number:         number,
		Title:          title,
		Body:           body,
		Files:          []string{},
		Labels:         []string{"autorelease: pending"},
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	// The payload arrives as raw JSON; do not HTML-escape it.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(encoded.String(), "\n")
}

type releasePleaseStep struct {
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Env  map[string]string `yaml:"env"`
	Run  string            `yaml:"run"`
}

func readReleasePleaseMergeStep(t *testing.T, dir string) releasePleaseStep {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(dir, releasePleaseWorkflowPath))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []releasePleaseStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(normalizeWorkflowLineEndings(string(contents))), &workflow); err != nil {
		t.Fatal(err)
	}
	for _, step := range workflow.Jobs["release-please"].Steps {
		if step.Name == releasePleaseMergeStepName {
			return step
		}
	}
	t.Fatalf("%s the release-please workflow must keep the %q step", releasePleasePolicyPrefix, releasePleaseMergeStepName)
	return releasePleaseStep{}
}

// TestReleasePleaseMergeStepNeverInterpolatesPayloadIntoShell pins the boundary
// that the v0.74.21 chain broke: the release PR payload reaches the shell as an
// environment variable, never as script text. The assertions run against the
// commands the shell would actually execute (comments and line continuations
// resolved), so a decoy that only mentions a command in a comment cannot pass.
func TestReleasePleaseMergeStepNeverInterpolatesPayloadIntoShell(t *testing.T) {
	step := readReleasePleaseMergeStep(t, ".")

	if got := step.Env[releasePleasePayloadEnvVar]; got != "${{ steps.rp.outputs.pr }}" {
		t.Fatalf("%s the auto-merge step must receive the release PR payload as %s, got %q",
			releasePleasePolicyPrefix, releasePleasePayloadEnvVar, got)
	}

	commands := releasePleaseShellCommands(step.Run)
	// Any expression rendered into `run` becomes script text before the shell
	// parses it, so any expression here can turn payload data into shell code.
	for _, command := range commands {
		if strings.Contains(command, "${{") {
			t.Fatalf("%s the auto-merge step must not interpolate expressions into its shell script, found %q", releasePleasePolicyPrefix, command)
		}
	}

	// The payload is parsed out of the environment as data, and the merge stays
	// synchronous: the rerun dispatch below only makes sense once the release
	// commit is on master (`--auto` would return before GitHub merged the release
	// PR and release-please would re-read master too early). The release branch
	// carries no required checks to wait for; the published artifacts are
	// verified by release-publish.yml instead.
	wantCommands := []string{
		`PR_NUMBER=$(printf '%s' "$RP_PR" | jq -r '.number')`,
		`gh pr merge "$PR_NUMBER" --repo "$GITHUB_REPOSITORY" --merge`,
		`gh api --method POST "/repos/$GITHUB_REPOSITORY/dispatches" -f event_type=release-please-rerun`,
	}
	if !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("%s the auto-merge step must run exactly:\n%q\nwant:\n%q", releasePleasePolicyPrefix, commands, wantCommands)
	}
}

// releasePleaseShellCommands returns the commands the shell would execute for a
// step `run` block: whole-line and trailing comments dropped, line continuations
// joined, whitespace collapsed.
func releasePleaseShellCommands(run string) []string {
	var commands []string
	joined := ""
	for _, line := range strings.Split(run, "\n") {
		trimmed := strings.TrimSpace(releasePleaseUncommentedShell(line))
		if trimmed == "" {
			continue
		}
		continued := strings.HasSuffix(trimmed, "\\")
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "\\"))
		if joined != "" {
			joined += " "
		}
		joined += trimmed
		if !continued {
			commands = append(commands, strings.Join(strings.Fields(joined), " "))
			joined = ""
		}
	}
	if joined != "" {
		commands = append(commands, strings.Join(strings.Fields(joined), " "))
	}
	return commands
}

// releasePleaseUncommentedShell drops a shell comment from a line, honouring
// single and double quotes so a `#` inside a quoted argument survives.
func releasePleaseUncommentedShell(line string) string {
	var quote rune
	for index, char := range line {
		switch {
		case quote != 0:
			if char == quote {
				quote = 0
			}
		case char == '\'' || char == '"':
			quote = char
		case char == '#':
			return line[:index]
		}
	}
	return line
}

var releasePleaseExpression = regexp.MustCompile(`\$\{\{[^}]*\}\}`)

// renderReleasePleaseStepText resolves the GitHub expressions the runner resolves
// before the shell ever sees the script. It deliberately substitutes the release
// PR payload as *text* for `${{ steps.rp.outputs.pr }}`: a workflow that
// interpolates the payload into `run` must reproduce the 2026-09-27 failure.
func renderReleasePleaseStepText(text, payload string) string {
	return releasePleaseExpression.ReplaceAllStringFunc(text, func(expression string) string {
		switch strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expression, "${{"), "}}")) {
		case "steps.rp.outputs.pr":
			return payload
		case "github.repository":
			return "minekube/gate"
		default:
			// The stub `gh` never reads GH_TOKEN, so any remaining expression
			// (e.g. `secrets.GITHUB_TOKEN`) only needs a non-empty value.
			return "stub"
		}
	})
}

// runReleasePleaseMergeStep executes the step's own shell script the way the
// runner does (bash -e) with a stub `gh` on PATH, and returns the combined
// script output plus the recorded `gh` invocations.
func runReleasePleaseMergeStep(t *testing.T, run string, stepEnv map[string]string, payload string) (string, error) {
	t.Helper()

	if runtime.GOOS == "windows" {
		// The auto-merge step only ever runs on ubuntu-latest, and Git Bash cannot
		// execute the Windows temp path this harness would hand it. The payload
		// policy assertions above (and the mutation harness) cover every platform.
		t.Skip("the auto-merge step is a POSIX-shell step; nothing to prove on Windows")
	}
	shell, err := exec.LookPath("bash")
	if err != nil {
		shell, err = exec.LookPath("sh")
	}
	if err != nil {
		t.Skipf("no bash/sh on PATH; the auto-merge step is a bash step (the runner executes it with /usr/bin/bash -e): %v", err)
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skipf("the auto-merge step needs jq and it is not on PATH: %v", err)
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ghLog := filepath.Join(dir, "gh.log")
	// The stub records every gh invocation so the assertions can tell a real
	// `gh pr merge <number>` / `gh api ... event_type=release-please-rerun` call
	// from a shell that never got that far, in either order.
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$GH_LOG\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(dir, "release-please-auto-merge.sh")
	if err := os.WriteFile(script, []byte(renderReleasePleaseStepText(run, payload)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"PATH=" + binDir + string(filepath.ListSeparator) + os.Getenv("PATH"),
		"GH_LOG=" + ghLog,
		"GITHUB_REPOSITORY=minekube/gate",
	}
	for name, value := range stepEnv {
		env = append(env, name+"="+renderReleasePleaseStepText(value, payload))
	}

	cmd := exec.Command(shell, "-e", script)
	cmd.Dir = dir
	cmd.Env = env
	output, runErr := cmd.CombinedOutput()
	logged, err := os.ReadFile(ghLog)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(output) + "recorded gh calls:\n" + string(logged), runErr
}

// TestReleasePleaseMergeStepHandlesApostrophesInReleaseNotes runs the workflow's
// own auto-merge step against the payload that stalled v0.74.21 (its body repeats
// the merged subject `fix(gate): run Gate's ...`). An apostrophe in a merged
// commit subject is normal input, so the step must merge the release PR and
// dispatch the rerun for it.
func TestReleasePleaseMergeStepHandlesApostrophesInReleaseNotes(t *testing.T) {
	step := readReleasePleaseMergeStep(t, ".")

	cases := []struct {
		name      string
		payload   string
		wantMerge string
	}{
		{
			name:      "apostrophe-in-release-notes",
			payload:   releasePleasePayload(t, 1202, "chore(master): release 0.74.21", releasePleasePRBodyApostrophe),
			wantMerge: "pr merge 1202 --repo minekube/gate --merge",
		},
		{
			name:      "quote-free-release-notes",
			payload:   releasePleasePayload(t, 1203, "chore(master): release 0.74.22", releasePleasePRBodyQuoteFree),
			wantMerge: "pr merge 1203 --repo minekube/gate --merge",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			output, runErr := runReleasePleaseMergeStep(t, step.Run, step.Env, testCase.payload)
			if runErr != nil {
				t.Fatalf("the auto-merge step failed for a release PR payload with %s:\n%v\n%s", testCase.name, runErr, output)
			}
			for _, want := range []string{
				testCase.wantMerge,
				"api --method POST /repos/minekube/gate/dispatches -f event_type=release-please-rerun",
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("the auto-merge step did not run `gh %s`:\n%s", want, output)
				}
			}
		})
	}
}

// TestReleasePleaseDetectionCatchesShellInterpolatedPayload rebuilds the pre-fix
// step body (the payload interpolated into a single-quoted shell literal, exactly
// as run 36320821502 executed it) and shows the harness above catches it — the RED
// this change started from — while the same shape still works for quote-free
// release notes, which is why the defect was silent until an apostrophe shipped.
func TestReleasePleaseDetectionCatchesShellInterpolatedPayload(t *testing.T) {
	step := readReleasePleaseMergeStep(t, ".")

	const payloadCommand = `printf '%s' "$RP_PR"`
	if !strings.Contains(step.Run, payloadCommand) {
		t.Fatalf("%s the auto-merge step no longer contains %q; update this mutation", releasePleasePolicyPrefix, payloadCommand)
	}
	historicalRun := strings.Replace(step.Run, payloadCommand, `echo '${{ steps.rp.outputs.pr }}'`, 1)

	apostrophePayload := releasePleasePayload(t, 1202, "chore(master): release 0.74.21", releasePleasePRBodyApostrophe)
	output, runErr := runReleasePleaseMergeStep(t, historicalRun, step.Env, apostrophePayload)
	if runErr == nil {
		t.Fatalf("a shell-interpolated payload must fail; the harness reported success:\n%s", output)
	}
	// The shell never got as far as running the script: nothing was merged and no
	// rerun was dispatched. Assert on that, not on one bash wording — Linux bash
	// reports `syntax error near unexpected token `('` while macOS bash 3.2
	// reports `unexpected EOF while looking for matching `"'`.
	if loweredOutput := strings.ToLower(output); !strings.Contains(loweredOutput, "syntax error") && !strings.Contains(loweredOutput, "unexpected eof") {
		t.Fatalf("the shell-interpolated payload must fail while the shell parses the script, got %v:\n%s", runErr, output)
	}
	if !strings.HasSuffix(output, "recorded gh calls:\n") {
		t.Fatalf("the historical shape must not reach gh at all:\n%s", output)
	}

	quoteFreePayload := releasePleasePayload(t, 1203, "chore(master): release 0.74.22", releasePleasePRBodyQuoteFree)
	if output, runErr := runReleasePleaseMergeStep(t, historicalRun, step.Env, quoteFreePayload); runErr != nil {
		t.Fatalf("the historical shape must still work for quote-free release notes (the defect was silent); got %v:\n%s", runErr, output)
	}
}

type releasePleaseWorkflowMutation struct {
	id    string
	apply func([]byte) ([]byte, error)
}

func replaceReleasePleaseWorkflow(id string, anchor, replacement string) releasePleaseWorkflowMutation {
	return releasePleaseWorkflowMutation{id: id, apply: func(source []byte) ([]byte, error) {
		if count := bytes.Count(source, []byte(anchor)); count != 1 {
			return nil, fmt.Errorf("mutation %s: anchor %q matched %d times, want exactly once", id, anchor, count)
		}
		return bytes.Replace(source, []byte(anchor), []byte(replacement), 1), nil
	}}
}

// TestReleasePleasePayloadPolicyRejectsWeakeningMutations proves the payload
// policy test is load-bearing: every candidate below keeps the workflow
// YAML-parseable and must still be rejected by the real policy assertions.
func TestReleasePleasePayloadPolicyRejectsWeakeningMutations(t *testing.T) {
	if os.Getenv(releasePleaseMutationChildEnv) == "1" {
		t.Skip("mutation child runs only the payload policy test")
	}

	baseline, err := os.ReadFile(releasePleaseWorkflowPath)
	if err != nil {
		t.Fatal(err)
	}
	baseline = []byte(normalizeWorkflowLineEndings(string(baseline)))

	mutations := []releasePleaseWorkflowMutation{
		// The historical, defect-carrying form: the payload interpolated into a
		// single-quoted shell literal.
		replaceReleasePleaseWorkflow("shell-interpolated-payload", `printf '%s' "$RP_PR"`, `echo '${{ steps.rp.outputs.pr }}'`),
		replaceReleasePleaseWorkflow("single-quoted-payload-expression", `printf '%s' "$RP_PR"`, `printf '%s' '${{ steps.rp.outputs.pr }}'`),
		// Interpolation and semantics weakenings that keep the shape readable.
		replaceReleasePleaseWorkflow("unquoted-payload", `printf '%s' "$RP_PR"`, `printf '%s' $RP_PR`),
		replaceReleasePleaseWorkflow("payload-not-parsed-as-json", `printf '%s' "$RP_PR" | jq -r '.number'`, `printf '%s' "$RP_PR"`),
		replaceReleasePleaseWorkflow("payload-echo-inert", `printf '%s' "$RP_PR" | jq -r '.number'`, `echo "$RP_PR" | jq -r '.number'`),
		replaceReleasePleaseWorkflow("payload-env-removed", "          RP_PR: ${{ steps.rp.outputs.pr }}\n", ""),
		replaceReleasePleaseWorkflow("repository-expression-interpolated", `--repo "$GITHUB_REPOSITORY"`, `--repo "${{ github.repository }}"`),
		replaceReleasePleaseWorkflow("async-merge", "            --merge\n", "            --auto\n"),
		// Decoys: the merge only mentioned in a comment, or wrapped in `echo`.
		replaceReleasePleaseWorkflow("merge-in-comment-decoy", "          gh pr merge \"$PR_NUMBER\" \\\n", "          # gh pr merge \"$PR_NUMBER\" \\\n"),
		replaceReleasePleaseWorkflow("merge-echo-inert", "          gh pr merge \"$PR_NUMBER\" \\\n", "          echo gh pr merge \"$PR_NUMBER\" \\\n"),
		replaceReleasePleaseWorkflow("dispatch-in-comment-decoy", "          gh api \\\n", "          # gh api \\\n"),
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
			if err := os.MkdirAll(filepath.Join(tmp, filepath.Dir(releasePleaseWorkflowPath)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tmp, releasePleaseWorkflowPath), candidate, 0o644); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(executable, "-test.run=^TestReleasePleaseMergeStepNeverInterpolatesPayloadIntoShell$", "-test.count=1")
			cmd.Dir = tmp
			cmd.Env = append(os.Environ(), releasePleaseMutationChildEnv+"=1")
			output, runErr := cmd.CombinedOutput()
			if runErr == nil {
				t.Fatalf("weakening survived the payload policy test\n%s", output)
			}
			if !bytes.Contains(output, []byte(releasePleasePolicyPrefix)) {
				t.Fatalf("weakening failed for the wrong reason; want a policy assertion\n%s", output)
			}
			t.Logf("MUTATION_REJECTED|%s|reason=release-please-payload-policy", mutation.id)
		})
	}
}
