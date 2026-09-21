package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkflow(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ci.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func scan(t *testing.T, body string) ([]pin, error) {
	t.Helper()
	return scanWorkflow(writeWorkflow(t, body))
}

const prefix = `name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
`

func TestStepToolchainScopeUsesStructureNotNames(t *testing.T) {
	tests := []struct {
		name, steps string
		wantGTC     string
	}{
		{
			name: "unrelated later unnamed override does not leak backward",
			steps: `      - run: go run example.com/tool@v1.0.0 ./...
      - env:
          GOTOOLCHAIN: go1.28.1
        run: go vet ./...
`,
		},
		{
			name: "unrelated earlier unnamed override does not leak forward",
			steps: `      - env:
          GOTOOLCHAIN: go1.28.1
        run: go vet ./...
      - run: go run example.com/tool@v1.0.0 ./...
`,
		},
		{
			name: "own unnamed override applies",
			steps: `      - env:
          GOTOOLCHAIN: go1.28.1
        run: go run example.com/tool@v1.0.0 ./...
`,
			wantGTC: "go1.28.1",
		},
		{
			name: "duplicate names remain separate",
			steps: `      - name: duplicate
        env:
          GOTOOLCHAIN: go1.28.1
        run: go vet ./...
      - name: duplicate
        run: go run example.com/tool@v1.0.0 ./...
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pins, err := scan(t, prefix+tt.steps)
			if tt.wantGTC == "" {
				if err == nil || !strings.Contains(err.Error(), "before a deterministic Go toolchain") {
					t.Fatalf("got pins=%+v err=%v; want deterministic-toolchain failure", pins, err)
				}
				return
			}
			if err != nil || len(pins) != 1 || pins[0].Toolchain != tt.wantGTC {
				t.Fatalf("pins=%+v err=%v", pins, err)
			}
		})
	}
}

func TestSetupGoAppliesOnlyAfterItRuns(t *testing.T) {
	before := prefix + `      - run: go run example.com/tool@v1.0.0 ./...
      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
`
	if pins, err := scan(t, before); err == nil || !strings.Contains(err.Error(), "runs before") {
		t.Fatalf("pins=%+v err=%v; setup-go must not apply backward", pins, err)
	}
	after := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run example.com/tool@v1.0.0 ./...
`
	pins, err := scan(t, after)
	if err != nil || len(pins) != 1 || pins[0].Toolchain != "1.28.1" {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}
}

func TestOnlyExactSetupGoActionCounts(t *testing.T) {
	body := prefix + `      - uses: acme/not-a-setup-go-action@v1
        with:
          go-version: 1.28.1
      - run: go run example.com/tool@v1.0.0 ./...
`
	if pins, err := scan(t, body); err == nil || !strings.Contains(err.Error(), "before a deterministic") {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}
}

func TestOnlyEnvMappingsProvideGoToolchain(t *testing.T) {
	body := `name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    outputs:
      GOTOOLCHAIN: go1.28.1
    steps:
      - run: go run example.com/tool@v1.0.0 ./...
`
	if pins, err := scan(t, body); err == nil || !strings.Contains(err.Error(), "before a deterministic") {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}
}

func TestYAMLQuotedToolchainIsEquivalent(t *testing.T) {
	for _, value := range []string{"go1.28.1", `"go1.28.1"`, `'go1.28.1'`} {
		body := prefix + "      - env:\n          GOTOOLCHAIN: " + value + "\n        run: go run example.com/tool@v1.0.0 ./...\n"
		pins, err := scan(t, body)
		if err != nil || len(pins) != 1 || pins[0].Toolchain != "go1.28.1" {
			t.Fatalf("value=%s pins=%+v err=%v", value, pins, err)
		}
	}
}

func TestShellLexerFindsRealCommandsOnly(t *testing.T) {
	tests := []struct {
		name, run string
		want      int
		wantSpec  string
	}{
		{"plain", `go run example.com/tool@v1.0.0 ./...`, 1, "example.com/tool@v1.0.0"},
		{"line continuation", "go \\\n  run example.com/tool@v1.0.0 ./...", 1, "example.com/tool@v1.0.0"},
		{"install", `go install example.com/tool@v1.0.0`, 1, "example.com/tool@v1.0.0"},
		{"flags equals", `go run -mod=mod example.com/tool@v1.0.0`, 1, "example.com/tool@v1.0.0"},
		{"flags separate", `go run -mod mod example.com/tool@v1.0.0`, 1, "example.com/tool@v1.0.0"},
		{"pipe without whitespace", `go run example.com/tool@v1.0.0|tee /tmp/x`, 1, "example.com/tool@v1.0.0"},
		{"if condition", `if go run example.com/tool@v1.0.0; then echo ok; fi`, 1, "example.com/tool@v1.0.0"},
		{"command wrapper", `command go install example.com/tool@v1.0.0`, 1, "example.com/tool@v1.0.0"},
		{"env wrapper", `env GOTOOLCHAIN=go1.28.1 go run example.com/tool@v1.0.0`, 1, "example.com/tool@v1.0.0"},
		{"comment", `echo done # go run example.com/ghost@v9.9.9`, 0, ""},
		{"echo text", `echo go run example.com/ghost@v9.9.9`, 0, ""},
		{"quoted text", `printf '%s' 'go run example.com/ghost@v9.9.9'`, 0, ""},
		{"local file", `go run .github/scripts/tool.go`, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pins, err := goCommands(tt.run)
			if err != nil || len(pins) != tt.want {
				t.Fatalf("pins=%+v err=%v want=%d", pins, err, tt.want)
			}
			if tt.want != 0 && pins[0].spec != tt.wantSpec {
				t.Fatalf("spec=%q want=%q", pins[0].spec, tt.wantSpec)
			}
		})
	}
}

func TestInlineCommentsAndWithArgsAreNotCommands(t *testing.T) {
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - uses: acme/an-action@v1
        with:
          args: go run example.com/ghost@v9.9.9 ./...
      - run: echo done # go run example.com/ghost@v9.9.9 ./...
      - run: go run example.com/real@v1.0.0 ./...
`
	pins, err := scan(t, body)
	if err != nil || len(pins) != 1 || pins[0].Spec != "example.com/real@v1.0.0" {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}
}

func TestPinStepNameMayAppearAfterRunInYAML(t *testing.T) {
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run example.com/tool@v1.0.0 ./...
        name: named later
`
	pins, err := scan(t, body)
	if err != nil || len(pins) != 1 || pins[0].Step != "named later" {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}
}

func TestNestedSequenceCannotBecomeAStep(t *testing.T) {
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
          matrix:
            - GOTOOLCHAIN: go9.9.9
      - run: go run example.com/tool@v1.0.0 ./...
`
	pins, err := scan(t, body)
	if err != nil || len(pins) != 1 || pins[0].Toolchain != "1.28.1" {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}
}

func TestCRLFNoTrailingNewlineAndSecondJob(t *testing.T) {
	body := `name: ci
on: push
jobs:
  first:
    runs-on: ubuntu-latest
    steps:
      - run: echo fine
  second:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run example.com/tool@v1.0.0 ./...`
	body = strings.ReplaceAll(body, "\n", "\r\n")
	pins, err := scan(t, body)
	if err != nil || len(pins) != 1 || pins[0].Job != "second" {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}
}

func TestToolchainPrecedence(t *testing.T) {
	body := `name: ci
env:
  GOTOOLCHAIN: go1.24.0
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    env:
      GOTOOLCHAIN: go1.25.0
    steps:
      - uses: actions/setup-go@v6
        with:
          go-version: 1.23.0
      - run: go run example.com/job@v1.0.0
      - env:
          GOTOOLCHAIN: go1.26.0
        run: go run example.com/step@v1.0.0
      - env:
          GOTOOLCHAIN: go1.26.0
        run: GOTOOLCHAIN=go1.27.0 go run example.com/inline@v1.0.0
`
	pins, err := scan(t, body)
	if err != nil || len(pins) != 3 {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}
	want := []string{"go1.25.0", "go1.26.0", "go1.27.0"}
	for i := range want {
		if pins[i].Toolchain != want[i] {
			t.Errorf("pin %d toolchain=%s want=%s", i, pins[i].Toolchain, want[i])
		}
	}
}

func TestSetupGoVersionFileMustExistAndHaveDirective(t *testing.T) {
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version-file: missing.mod
      - run: go run example.com/tool@v1.0.0
`
	if _, err := scan(t, body); err == nil || !strings.Contains(err.Error(), "missing.mod") {
		t.Fatalf("err=%v", err)
	}
}

func TestFloatingAndUnpinnedRefsFailClosed(t *testing.T) {
	for _, spec := range []string{"example.com/tool@latest", "example.com/tool@main", "example.com/tool@deadbeef", "example.com/tool"} {
		body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run ` + spec + "\n"
		if pins, err := scan(t, body); err == nil {
			t.Fatalf("spec=%s pins=%+v unexpectedly passed", spec, pins)
		}
	}
}

func TestCheckAllFailsWhenNoPins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(prefix+"      - run: echo fine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkAll(dir, func(string) (string, error) { return "1.0", nil }, os.Stdout); err == nil || !strings.Contains(err.Error(), "no pinned") {
		t.Fatalf("err=%v", err)
	}
}

func TestMalformedYAMLAndShellFailClosed(t *testing.T) {
	if _, err := scan(t, "jobs: ["); err == nil {
		t.Fatal("malformed YAML passed")
	}
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: "go run 'example.com/tool@v1.0.0"
`
	if _, err := scan(t, body); err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("err=%v", err)
	}
}

func TestModuleProxyResolverUsesModMetadataWithoutBuildingTool(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/github.com/kojah/gohawk/@v/v0.3.2.mod" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("module github.com/kojah/gohawk\n\ngo 1.27.0\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	oldProxy := moduleProxy
	moduleProxy = server.URL
	t.Cleanup(func() { moduleProxy = oldProxy })

	got, err := resolveModuleRequirement("github.com/kojah/gohawk/cmd/gohawk@v0.3.2")
	if err != nil || got != "1.27.0" {
		t.Fatalf("got=%q err=%v paths=%v", got, err, paths)
	}
	if len(paths) < 2 || paths[len(paths)-1] != "/github.com/kojah/gohawk/@v/v0.3.2.mod" {
		t.Fatalf("package-to-module fallback paths=%v", paths)
	}
	if got := escapeModulePath("Example.COM/Mod!"); got != "!example.!c!o!m/!mod!!" {
		t.Fatalf("escaped module path=%q", got)
	}
}

func TestVersionComparison(t *testing.T) {
	for _, tt := range []struct {
		have, need string
		want       bool
	}{{"1.27.1", "1.27.0", true}, {"go1.27.0", "1.27", true}, {"1.26.9", "1.27.0", false}, {"1.28.0", "1.27.9", true}} {
		gotVersion, err := concreteToolchain(tt.have)
		if err != nil {
			t.Fatal(err)
		}
		if got := versionAtLeast(gotVersion, normalizeVersion(tt.need)); got != tt.want {
			t.Errorf("%s >= %s = %v, want %v", tt.have, tt.need, got, tt.want)
		}
	}
	for _, bad := range []string{"auto", "local", "stable", "go1.27"} {
		if _, err := concreteToolchain(bad); err == nil {
			t.Errorf("%q unexpectedly concrete", bad)
		}
	}
}

func TestWorkflowShellASTFindsEveryDirectInvocation(t *testing.T) {
	tests := []struct {
		name string
		run  string
	}{
		{"command substitution", `result=$(go run example.com/high@v1.0.0)`},
		{"quoted command substitution", `result="$(go run example.com/high@v1.0.0)"`},
		{"backtick substitution", "result=`go run example.com/high@v1.0.0`"},
		{"prefix output redirection", `> /tmp/result go run example.com/high@v1.0.0`},
		{"prefix fd redirection", `2>/dev/null go run example.com/high@v1.0.0`},
		{"command end of options", `command -- go run example.com/high@v1.0.0`},
		{"exec argv zero", `exec -a checker go run example.com/high@v1.0.0`},
		{"env unset", `env -u GOPROXY go run example.com/high@v1.0.0`},
		{"function body", `function check_tool { go run example.com/high@v1.0.0; }; check_tool`},
		{"subshell", `(go run example.com/high@v1.0.0)`},
		{"command group", `{ go run example.com/high@v1.0.0; }`},
		{"pipeline", `printf ready | go run example.com/high@v1.0.0`},
		{"while condition", `while go run example.com/high@v1.0.0; do break; done`},
		{"until condition", `until go run example.com/high@v1.0.0; do break; done`},
		{"time clause", `time go run example.com/high@v1.0.0`},
		{"negated command", `! go run example.com/high@v1.0.0`},
		{"inline assignment", `GOTOOLCHAIN=go1.28.1 go run example.com/high@v1.0.0`},
		{"separate ldflags value", `go run -ldflags '-s -w' example.com/high@v1.0.0`},
		{"redirection suffix", `go run example.com/high@v1.0.0>/tmp/result`},
		{"escaped go command word", `\go run example.com/high@v1.0.0`},
		{"path qualified env wrapper", `/usr/bin/env go run example.com/high@v1.0.0`},
		{"env end of options with assignment", `env -- FOO=bar go run example.com/high@v1.0.0`},
		{"env end of options with toolchain", `env -- GOTOOLCHAIN=go1.28.1 go run example.com/high@v1.0.0`},
		{"env split string", `env -S 'go run example.com/high@v1.0.0'`},
		{"env split string with quotes", `env -S 'go run "example.com/high@v1.0.0"'`},
		{"env split string with assignment", `env -S 'FOO=bar go run example.com/high@v1.0.0'`},
		{"go chdir flag", `go -C sub run example.com/high@v1.0.0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variants := []struct {
				name string
				run  string
			}{
				{"A_plain", `go run example.com/high@v1.0.0`},
				{"B_construct", tt.run},
			}
			for _, variant := range variants {
				t.Run(variant.name, func(t *testing.T) {
					body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - name: two direct pins
        run: |
          go run example.com/ordinary@v1.0.0
          ` + variant.run + "\n"
					pins, err := scan(t, body)
					if err != nil {
						t.Fatalf("scan failed: %v", err)
					}
					if len(pins) != 2 {
						t.Fatalf("pins=%+v; want ordinary and high pins", pins)
					}
					if pins[0].Spec != "example.com/ordinary@v1.0.0" || pins[1].Spec != "example.com/high@v1.0.0" {
						t.Fatalf("specs=%q, %q", pins[0].Spec, pins[1].Spec)
					}
				})
			}
		})
	}
}

func TestWorkflowShellASTIgnoresDataAndLocalGoFiles(t *testing.T) {
	tests := []struct {
		name string
		run  string
	}{
		{"heredoc data", "cat <<'EOF'\ngo run example.com/ghost@v9.9.9\nEOF"},
		{"bare local Go file", `go run tool.go`},
		{"relative local Go file", `go run ./tool.go`},
		{"relative local package", `go run ./cmd/checker`},
		{"dot local package", `go run .`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: |
          go run example.com/ordinary@v1.0.0
          ` + strings.ReplaceAll(tt.run, "\n", "\n          ") + "\n"
			pins, err := scan(t, body)
			if err != nil || len(pins) != 1 || pins[0].Spec != "example.com/ordinary@v1.0.0" {
				t.Fatalf("pins=%+v err=%v", pins, err)
			}
		})
	}
}

func TestWorkflowShellASTRejectsUnknownGoFlags(t *testing.T) {
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run -definitely-unknown example.com/tool@v1.0.0
`
	if pins, err := scan(t, body); err == nil || !strings.Contains(err.Error(), "unsupported go run flag") {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}
}

func TestWorkflowShellASTHandlesGitHubExpressionsFailClosed(t *testing.T) {
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: |
          echo "repository=${{ github.repository }}"
          go run example.com/tool@v1.0.0
`
	pins, err := scan(t, body)
	if err != nil || len(pins) != 1 || pins[0].Spec != "example.com/tool@v1.0.0" {
		t.Fatalf("pins=%+v err=%v", pins, err)
	}

	dynamic := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run "${{ inputs.tool }}"
`
	if pins, err := scan(t, dynamic); err == nil || !strings.Contains(err.Error(), "dynamic shell word") {
		t.Fatalf("pins=%+v err=%v; dynamic direct target must fail closed", pins, err)
	}
}

func TestModulePinsRequireCompleteGoSemver(t *testing.T) {
	for _, version := range []string{"v1junk", "v1.2.3junk", "v1.2.3+", "v1.2.3-"} {
		t.Run("reject_"+version, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run example.com/tool@` + version + "\n"
			if pins, err := scan(t, body); err == nil || !strings.Contains(err.Error(), "valid tagged Go module version") {
				t.Fatalf("pins=%+v err=%v", pins, err)
			}
		})
	}
	for _, version := range []string{
		"v1", "v1.2", "v1.2.3", "v1.2.3-rc.1", "v1.2.3+build.7",
		"v0.0.0-20240101120000-abcdefabcdef", "v2.0.0+incompatible",
	} {
		t.Run("accept_"+version, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run example.com/tool@` + version + "\n"
			pins, err := scan(t, body)
			if err != nil || len(pins) != 1 || pins[0].Spec != "example.com/tool@"+version {
				t.Fatalf("pins=%+v err=%v", pins, err)
			}
		})
	}
}

func TestCheckAllFailsWhenRequirementResolutionFails(t *testing.T) {
	dir := t.TempDir()
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run example.com/tool@v1.0.0
`
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := os.CreateTemp(t.TempDir(), "check-output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	err = checkAll(dir, func(string) (string, error) { return "", errors.New("proxy unavailable") }, out)
	if err == nil || !strings.Contains(err.Error(), "1 of 1 pinned tool") {
		t.Fatalf("err=%v; resolver failure must fail the production check", err)
	}
}

// runCheckAll drives the production entry point and returns what it printed.
func runCheckAll(t *testing.T, body string, resolve requirementResolver) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := os.CreateTemp(t.TempDir(), "check-output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	checkErr := checkAll(dir, resolve, out)
	printed, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(printed), checkErr
}

// A pin that outgrew the toolchain its step runs with must fail the job, not
// just print a diagnostic: the printed FAIL line alone leaves CI green, which
// is the regression class this guard exists for.
func TestCheckAllFailsWhenToolchainIsTooLow(t *testing.T) {
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.26.0
      - run: go run example.com/tool@v1.0.0
`
	out, err := runCheckAll(t, body, func(string) (string, error) { return "1.28.0", nil })
	if err == nil || !strings.Contains(err.Error(), "1 of 1 pinned tool") {
		t.Fatalf("err=%v out=%s; a tool that outgrew its toolchain must fail the check", err, out)
	}
	if !strings.Contains(out, "requires go >= 1.28.0 but setup-go go-version=1.26.0 provides go 1.26.0") {
		t.Fatalf("out=%s; want the printed diagnostic for the too-low toolchain", out)
	}
}

// A toolchain the guard cannot pin to a concrete version must fail the job as
// well, for the same reason: the diagnostic is not the contract.
func TestCheckAllFailsWhenToolchainIsNotConcrete(t *testing.T) {
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - env:
          GOTOOLCHAIN: auto
        run: go run example.com/tool@v1.0.0
`
	out, err := runCheckAll(t, body, func(string) (string, error) { return "1.0.0", nil })
	if err == nil || !strings.Contains(err.Error(), "1 of 1 pinned tool") {
		t.Fatalf("err=%v out=%s; a non-concrete toolchain must fail the check", err, out)
	}
	if !strings.Contains(out, "GOTOOLCHAIN=auto is not an exact toolchain") {
		t.Fatalf("out=%s; want the printed diagnostic for the non-concrete toolchain", out)
	}
}

// End-to-end regression restored from the deleted Bash self-test (case 5): a
// job-level toolchain that is too old for the pin fails the check, and the
// diagnostic names that job-level source.
func TestCheckAllFailsWhenJobToolchainIsTooLow(t *testing.T) {
	body := `name: ci
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    env:
      GOTOOLCHAIN: go1.22.0
    steps:
      - run: go run example.com/tool@v1.0.0 ./...
`
	out, err := runCheckAll(t, body, func(string) (string, error) { return "1.23.0", nil })
	if err == nil || !strings.Contains(err.Error(), "1 of 1 pinned tool") {
		t.Fatalf("err=%v out=%s; a job-level toolchain that is too old must fail the check", err, out)
	}
	if !strings.Contains(out, "FAIL") || !strings.Contains(out, "job env GOTOOLCHAIN=go1.22.0 provides go 1.22.0") {
		t.Fatalf("out=%s; want the job-level diagnostic", out)
	}
}

// The GOTOOLCHAIN an `env` wrapper assigns is the toolchain the step really
// runs the tool with, so it is the pin's classification source and outranks
// setup-go's version - both before and after env's `--`.
func TestCheckAllUsesEnvWrapperToolchain(t *testing.T) {
	for _, construct := range []string{
		`env GOTOOLCHAIN=go1.28.1 go run example.com/tool@v1.0.0`,
		`env -- GOTOOLCHAIN=go1.28.1 go run example.com/tool@v1.0.0`,
	} {
		t.Run(construct, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.26.0
      - run: ` + construct + "\n"
			pins, err := scan(t, body)
			if err != nil || len(pins) != 1 {
				t.Fatalf("pins=%+v err=%v", pins, err)
			}
			if pins[0].Toolchain != "go1.28.1" || pins[0].Source != "inline GOTOOLCHAIN=go1.28.1" {
				t.Fatalf("pin=%+v; the env wrapper's GOTOOLCHAIN must be the classification source", pins[0])
			}
			if _, err := runCheckAll(t, body, func(string) (string, error) { return "1.28.0", nil }); err != nil {
				t.Fatalf("err=%v; the wrapper's exact toolchain satisfies the requirement", err)
			}
		})
	}
}

// Static text handed to the job's own shell (eval, `sh -c '...'`) executes in
// the same job, so pins inside it must be verified. Each row carries an
// ordinary pin as well, so a zero-pin guard cannot hide an omission.
func TestWorkflowShellASTVerifiesThroughEvalAndChildShells(t *testing.T) {
	tests := []struct {
		name, construct string
	}{
		{"eval static text", `eval "go run example.com/high@v1.0.0"`},
		{"child shell -c static text", `bash -c 'go run example.com/high@v1.0.0'`},
		{"posix shell -c static text", `sh -c 'go run example.com/high@v1.0.0'`},
		{"nested eval", `eval 'eval "go run example.com/high@v1.0.0"'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variants := []struct {
				name string
				run  string
			}{
				{"A_plain", `go run example.com/high@v1.0.0`},
				{"B_construct", tt.construct},
			}
			for _, variant := range variants {
				t.Run(variant.name, func(t *testing.T) {
					body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - name: two direct pins
        run: |
          go run example.com/ordinary@v1.0.0
          ` + variant.run + "\n"
					pins, err := scan(t, body)
					if err != nil {
						t.Fatalf("scan failed: %v", err)
					}
					if len(pins) != 2 {
						t.Fatalf("pins=%+v; want ordinary and high pins", pins)
					}
					if pins[0].Spec != "example.com/ordinary@v1.0.0" || pins[1].Spec != "example.com/high@v1.0.0" {
						t.Fatalf("specs=%q, %q", pins[0].Spec, pins[1].Spec)
					}
				})
			}
		})
	}
}

// A launcher executes the command word it is handed, so that command word is
// classified through the same path as a top-level one. Each row carries an
// ordinary pin as well, so a zero-pin guard cannot hide an omission.
func TestWorkflowShellASTVerifiesThroughLaunchers(t *testing.T) {
	tests := []struct {
		name, construct string
	}{
		{"nohup launcher", `nohup go run example.com/high@v1.0.0`},
		{"timeout launcher", `timeout 60 go run example.com/high@v1.0.0`},
		{"xargs launcher", `xargs go run example.com/high@v1.0.0`},
		{"sudo launcher", `sudo go run example.com/high@v1.0.0`},
		{"sudo user value", `sudo -u checker go run example.com/high@v1.0.0`},
		{"sudo long option", `sudo --preserve-env go run example.com/high@v1.0.0`},
		{"xargs replace value", `xargs -I {} go run example.com/high@v1.0.0`},
		{"timeout kill-after value", `timeout -k 5s 60 go run example.com/high@v1.0.0`},
		{"dynamic duration operand", `timeout $DURATION go run example.com/high@v1.0.0`},
		{"find -exec launcher", `find . -name x -exec go run example.com/high@v1.0.0`},
		{"nohup child shell", `nohup sh -c 'go run example.com/high@v1.0.0'`},
		{"timeout child shell", `timeout 30 bash -c 'go run example.com/high@v1.0.0'`},
		{"su command flag", `su checker -c 'go run example.com/high@v1.0.0'`},
		{"flock command flag", `flock -x /tmp/gate.lock -c 'go run example.com/high@v1.0.0'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - name: two direct pins
        run: |
          go run example.com/ordinary@v1.0.0
          ` + tt.construct + "\n"
			pins, err := scan(t, body)
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}
			if len(pins) != 2 {
				t.Fatalf("pins=%+v; want ordinary and high pins", pins)
			}
			if pins[0].Spec != "example.com/ordinary@v1.0.0" || pins[1].Spec != "example.com/high@v1.0.0" {
				t.Fatalf("specs=%q, %q", pins[0].Spec, pins[1].Spec)
			}
		})
	}
}

// Anything the guard cannot resolve statically must fail the workflow open
// instead of dropping a possible pin from the count. A launcher whose operand
// is dynamic and followed by run/install is the launcher form of the same rule.
func TestWorkflowShellASTFailsClosedOnUnverifiableShellDelegation(t *testing.T) {
	tests := []struct {
		name, construct, want string
	}{
		{"dynamic eval text", `eval "${{ inputs.script }}"`, "eval argument cannot be statically verified"},
		{"dynamic child shell text", `bash -c "${{ inputs.script }}"`, "script cannot be statically verified"},
		{"child shell script file", `bash .github/scripts/release.sh`, "script file"},
		{"unknown command wrapper flag", `command -Z go run example.com/tool@v1.0.0`, "unsupported command wrapper flag"},
		{"unknown exec wrapper flag", `exec -Z go run example.com/tool@v1.0.0`, "unsupported exec wrapper flag"},
		{"unknown env wrapper flag", `env -Z go run example.com/tool@v1.0.0`, "unsupported env wrapper flag"},
		{"dynamic nohup operand", `nohup $GO run example.com/tool@v1.0.0`, "dynamic command name"},
		{"dynamic timeout operand", `timeout 30 $GO run example.com/tool@v1.0.0`, "dynamic command name"},
		{"dynamic xargs operand", `xargs -I {} $GO run example.com/tool@v1.0.0`, "dynamic command name"},
		{"dynamic find -exec operand", `find . -exec $GO run example.com/tool@v1.0.0 {} \;`, "dynamic command name"},
		{"dynamic env split string", `env -S "${{ inputs.split }}"`, "dynamic shell word"},
		{"dynamic launcher value", `sudo -u "${{ inputs.user }}" go run example.com/tool@v1.0.0`, "dynamic shell word"},
		{"launcher argument spells a go invocation", `nohup echo go run example.com/tool@v1.0.0`, "cannot be verified statically"},
		{"launcher operand dynamic before run", `nohup echo $TOOL run example.com/tool@v1.0.0`, "cannot be verified statically"},
		{"dynamic launcher operand before run", `timeout $TOOL run example.com/tool@v1.0.0`, "cannot be verified statically"},
		{"dynamic command word", `$LAUNCHER go run example.com/tool@v1.0.0`, "dynamic command name"},
		{"dynamic command word with go subcommand", `"$CMD" run example.com/tool@v1.0.0`, "dynamic command name"},
		{"dynamic trap action", `trap "${{ inputs.cmd }}" EXIT`, "trap argument cannot be statically verified"},
		{"unquoted trap action spells a go invocation", `trap go run example.com/tool@v1.0.0 EXIT`, "cannot be verified statically"},
		{"unmodelled executor handed a go invocation", `someci-tool go run example.com/tool@v1.0.0`, "cannot be verified statically"},
		{"unmodelled executor with a dynamic command word", `someci-tool $CMD run example.com/tool@v1.0.0`, "cannot be verified statically"},
		{"expect spawn text", `expect -c 'spawn go run example.com/tool@v1.0.0'`, "cannot be verified statically"},
		{"script dynamic command text", `script -c "${{ inputs.cmd }}" /dev/null`, "command cannot be statically verified"},
		{"at reads its script from stdin", `echo 'go run example.com/tool@v1.0.0' | at now`, "reads the script it runs from standard input"},
		{"batch reads its script from stdin", `echo 'go run example.com/tool@v1.0.0' | batch`, "reads the script it runs from standard input"},
		{"dynamic go -C directory", `go -C $DIR run example.com/tool@v1.0.0`, "go -C requires a directory"},
		{"install flag after a package", `go install example.com/tool@v1.0.0 -u`, "appears after a package"},
		{"go subcommand without a package", `go run`, "has no package argument"},
		{"child shell without a script", `bash -c`, "requires a script"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: |
          ` + tt.construct + "\n"
			pins, err := scan(t, body)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("pins=%+v err=%v; want error containing %q", pins, err, tt.want)
			}
		})
	}
}

// Nested shell text is recursed into, but not without bound: past the depth
// bound the guard fails closed instead of recursing further.
func TestNestedShellTextDepthFailsClosed(t *testing.T) {
	const src = `go run example.com/high@v1.0.0`
	if pins, err := scanStaticShellText(src, 1, "", 7); err != nil || len(pins) != 1 {
		t.Fatalf("pins=%+v err=%v; nesting within the bound must still be verified", pins, err)
	}
	if pins, err := scanStaticShellText(src, 1, "", 8); err == nil || !strings.Contains(err.Error(), "nested too deeply") {
		t.Fatalf("pins=%+v err=%v; nesting beyond the depth bound must fail closed", pins, err)
	}
}

// The fail-closed rules must stay narrow: text that cannot contain a Go tool
// invocation is still ignored instead of erroring.
func TestWorkflowShellASTKeepsUnrelatedShellTextIgnored(t *testing.T) {
	tests := []struct {
		name, run string
	}{
		{"launcher without a go tool", `timeout 5 curl -sS https://proxy.golang.org/`},
		{"dynamic command without run subcommand", `$COMPILER build ./...`},
		{"child shell without a go tool", `bash -c 'echo done'`},
		{"static eval without a go tool", `eval "echo done"`},
		{"echoed go invocation text", `echo go run example.com/ghost@v9.9.9`},
		{"which go", `which go`},
		{"printf with unquoted go text", `printf '%s\n' go run example.com/ghost@v9.9.9`},
		{"npm script step", `npm run build`},
		{"docker run with a go image", `docker run --rm golang:1.26 go version`},
		{"make target", `make test`},
		{"git subcommand", `git bisect reset`},
		{"unknown tool without a go invocation", `someci-tool --flag value ./src`},
		{"unknown tool with a run argument", `someci-tool run build --release`},
		{"trap that only echoes", `trap 'echo done' EXIT`},
		{"trap listing signals", `trap -l`},
		{"trap reset", `trap - EXIT`},
		{"local package through a child shell", `bash -c 'go run ./cmd/checker'`},
		{"go test and build", `go test ./... && go build ./cmd/gate`},
		{"local go file through env", `env go run tool.go`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pins, err := goCommands(tt.run)
			if err != nil || len(pins) != 0 {
				t.Fatalf("pins=%+v err=%v; want no pins and no error", pins, err)
			}
		})
	}
}

// A dynamic inline GOTOOLCHAIN assignment in front of a Go invocation is a
// boundary this guard cannot resolve, so the whole invocation must fail closed
// instead of being dropped from the pin count.
func TestWorkflowShellASTFailsClosedOnDynamicGoToolchainAssignment(t *testing.T) {
	tests := []struct {
		name, construct, want string
	}{
		{"matrix expression", `GOTOOLCHAIN=${{ matrix.go }} go run example.com/high@v1.0.0`, "dynamic GOTOOLCHAIN assignment cannot be verified"},
		{"shell variable", `GOTOOLCHAIN=$GTC go run example.com/high@v1.0.0`, "dynamic GOTOOLCHAIN assignment cannot be verified"},
		{"dynamic assignment inside go install", `GOTOOLCHAIN=${{ matrix.go }} go install example.com/high@v1.0.0`, "dynamic GOTOOLCHAIN assignment cannot be verified"},
		{"env wrapper carries the assignment", `env GOTOOLCHAIN=${{ matrix.go }} go run example.com/high@v1.0.0`, "dynamic shell word"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: |
          ` + tt.construct + "\n"
			pins, err := scan(t, body)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("pins=%+v err=%v; want error containing %q", pins, err, tt.want)
			}
		})
	}

	// The static counterpart still resolves, so the rule above is about the
	// dynamic boundary and not about inline assignments in general.
	body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: GOTOOLCHAIN=go1.28.1 go run example.com/high@v1.0.0
`
	pins, err := scan(t, body)
	if err != nil || len(pins) != 1 || pins[0].Toolchain != "go1.28.1" {
		t.Fatalf("pins=%+v err=%v; a static inline assignment must still verify", pins, err)
	}
}

// A child interpreter whose own argument words are dynamic cannot be resolved:
// the script it would run must fail closed rather than be skipped as if the
// interpreter ran no Go tool.
func TestWorkflowShellASTFailsClosedOnDynamicChildShellArgument(t *testing.T) {
	tests := []struct {
		name, construct, want string
	}{
		{"dynamic option word", `bash $OPTS -c 'go run example.com/high@v1.0.0'`, "argument cannot be statically verified"},
		{"dynamic option word before a script file", `bash $OPTS .github/scripts/release.sh`, "argument cannot be statically verified"},
		{"matrix option word", `bash "${{ inputs.opts }}" -c 'go run example.com/high@v1.0.0'`, "argument cannot be statically verified"},
		{"posix shell dynamic option word", `sh $OPTS -c 'go run example.com/high@v1.0.0'`, "argument cannot be statically verified"},
		{"nested dynamic option word", `bash -c 'bash $OPTS -c "go run example.com/high@v1.0.0"'`, "argument cannot be statically verified"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: |
          ` + tt.construct + "\n"
			pins, err := scan(t, body)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("pins=%+v err=%v; want error containing %q", pins, err, tt.want)
			}
		})
	}
}

// Every workflow file in the directory contributes its pins: a whole-file
// omission is invisible when each fixture lives in its own t.TempDir().
func TestCheckAllReportsPinsFromEveryWorkflowFile(t *testing.T) {
	dir := t.TempDir()
	specs := map[string]string{
		"a.yml":  "example.com/alpha@v1.0.0",
		"b.yml":  "example.com/beta@v1.0.0",
		"c.yaml": "example.com/gamma@v1.0.0",
	}
	for name, spec := range specs {
		body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - run: go run ` + spec + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, err := os.CreateTemp(t.TempDir(), "check-output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := checkAll(dir, func(string) (string, error) { return "1.0.0", nil }, out); err != nil {
		t.Fatalf("checkAll over three workflow files: %v", err)
	}
	printed, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	text := string(printed)
	if !strings.Contains(text, "3 pinned tool(s) verified") {
		t.Fatalf("output=%q; want every workflow file's pin reported", text)
	}
	for name, spec := range specs {
		if !strings.Contains(text, name) || !strings.Contains(text, spec) {
			t.Fatalf("output=%q; missing %s (%s)", text, name, spec)
		}
	}
}

func TestWorkflowFilesIncludesEveryMatch(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.yml", "b.yml", "c.yaml", "d.yaml", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("name: ci\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := workflowFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(dir, "a.yml"),
		filepath.Join(dir, "b.yml"),
		filepath.Join(dir, "c.yaml"),
		filepath.Join(dir, "d.yaml"),
	}
	if len(files) != len(want) {
		t.Fatalf("files=%v; want every match of both extensions", files)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("files=%v; want %v", files, want)
		}
	}
}

// trap runs the shell text it is handed, so that text is scanned like any other
// static text the job's own shell executes.
func TestWorkflowShellASTVerifiesThroughTrap(t *testing.T) {
	tests := []struct {
		name, construct string
	}{
		{"single quoted action", `trap 'go run example.com/high@v1.0.0' EXIT`},
		{"double quoted action", `trap "go run example.com/high@v1.0.0" EXIT`},
		{"several signal specs", `trap 'go run example.com/high@v1.0.0' EXIT INT TERM`},
		{"inside a child shell", `bash -c 'trap "go run example.com/high@v1.0.0" EXIT'`},
		{"inside eval", `eval 'trap "go run example.com/high@v1.0.0" EXIT'`},
		{"behind a launcher", `nohup trap 'go run example.com/high@v1.0.0' EXIT`},
		{"nested shell text", `trap 'bash -c "go run example.com/high@v1.0.0"' EXIT`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - name: two direct pins
        run: |
          go run example.com/ordinary@v1.0.0
          ` + tt.construct + "\n"
			pins, err := scan(t, body)
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}
			if len(pins) != 2 {
				t.Fatalf("pins=%+v; want ordinary and high pins", pins)
			}
			if pins[0].Spec != "example.com/ordinary@v1.0.0" || pins[1].Spec != "example.com/high@v1.0.0" {
				t.Fatalf("specs=%q, %q", pins[0].Spec, pins[1].Spec)
			}
		})
	}
}

// script -c and expect -c carry their command as shell text, exactly like su -c
// and flock -c, so the text is scanned instead of being dropped.
func TestWorkflowShellASTVerifiesThroughScriptAndExpect(t *testing.T) {
	tests := []struct {
		name, construct string
	}{
		{"script -c", `script -c 'go run example.com/high@v1.0.0' /dev/null`},
		{"script -q -c", `script -q -c 'go run example.com/high@v1.0.0' /dev/null`},
		{"expect -c", `expect -c 'go run example.com/high@v1.0.0'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - name: two direct pins
        run: |
          go run example.com/ordinary@v1.0.0
          ` + tt.construct + "\n"
			pins, err := scan(t, body)
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}
			if len(pins) != 2 {
				t.Fatalf("pins=%+v; want ordinary and high pins", pins)
			}
			if pins[0].Spec != "example.com/ordinary@v1.0.0" || pins[1].Spec != "example.com/high@v1.0.0" {
				t.Fatalf("specs=%q, %q", pins[0].Spec, pins[1].Spec)
			}
		})
	}
}

// A bare `--` ends a launcher's option list; it takes no value, so the command
// word behind it must be classified instead of being skipped as an operand.
func TestWorkflowShellASTVerifiesLauncherEndOfOptions(t *testing.T) {
	tests := []struct {
		name, construct string
	}{
		{"nohup end of options", `nohup -- go run example.com/high@v1.0.0`},
		{"sudo end of options", `sudo -- go run example.com/high@v1.0.0`},
		{"timeout end of options", `timeout -- 30 go run example.com/high@v1.0.0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := prefix + `      - uses: actions/setup-go@v6
        with:
          go-version: 1.28.1
      - name: two direct pins
        run: |
          go run example.com/ordinary@v1.0.0
          ` + tt.construct + "\n"
			pins, err := scan(t, body)
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}
			if len(pins) != 2 {
				t.Fatalf("pins=%+v; want ordinary and high pins", pins)
			}
		})
	}
}

// GitHub expands ${{ ... }} before the shell runs, so a truncated expression
// cannot be resolved to the same runtime boundary and must fail closed instead
// of being left as literal shell text.
func TestShellWithGitHubExpressionsRejectsUnterminatedExpression(t *testing.T) {
	if _, err := shellWithGitHubExpressions(`echo "${{ inputs.script"`); err == nil || !strings.Contains(err.Error(), "unterminated GitHub Actions expression") {
		t.Fatalf("err=%v; want an unterminated-expression error", err)
	}
	out, err := shellWithGitHubExpressions(`echo "${{ github.repository }}"`)
	if err != nil || !strings.Contains(out, "GITHUB_ACTIONS_EXPR") {
		t.Fatalf("out=%q err=%v; want the expression replaced with a dynamic parameter", out, err)
	}
}
