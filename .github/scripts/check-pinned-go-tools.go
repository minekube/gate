// Command check-pinned-go-tools verifies that every `go run` or `go install`
// module pin in GitHub Actions runs with a compatible, deterministic Go
// toolchain. It parses workflows as YAML (rather than approximating YAML with
// line/indent matching) and models setup-go in execution order.
package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/syntax"
)

type workflow struct {
	Env  map[string]any `yaml:"env"`
	Jobs map[string]job `yaml:"jobs"`
}

type job struct {
	Env   map[string]any `yaml:"env"`
	Steps []step         `yaml:"steps"`
}

type step struct {
	Name string         `yaml:"name"`
	Uses string         `yaml:"uses"`
	Env  map[string]any `yaml:"env"`
	With map[string]any `yaml:"with"`
	Run  string         `yaml:"run"`
}

type pin struct {
	File, Job, Step, Spec, Toolchain, Source string
	Line                                     int
}

type requirementResolver func(spec string) (string, error)

var concreteGo = regexp.MustCompile(`^(?:go)?[0-9]+\.[0-9]+\.[0-9]+$`)

func main() {
	dir := ".github/workflows"
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: check-pinned-go-tools.go [workflow-dir]")
		os.Exit(2)
	}
	if len(os.Args) == 2 {
		dir = os.Args[1]
	}
	reqs := parseRequirementOverride(os.Getenv("CHECK_PINNED_TOOLS_REQUIREMENTS"))
	resolve := resolveModuleRequirement
	if reqs != nil {
		resolve = func(spec string) (string, error) {
			v, ok := reqs[spec]
			if !ok {
				return "", fmt.Errorf("no test requirement for %s", spec)
			}
			return v, nil
		}
	}
	if err := checkAll(dir, resolve, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkAll(dir string, resolve requirementResolver, out *os.File) error {
	files, err := workflowFiles(dir)
	if err != nil {
		return err
	}
	var pins []pin
	for _, file := range files {
		p, err := scanWorkflow(file)
		if err != nil {
			return err
		}
		pins = append(pins, p...)
	}
	if len(pins) == 0 {
		return fmt.Errorf("check-pinned-go-tools: no pinned Go tools found in %s", dir)
	}
	failures := 0
	for _, p := range pins {
		required, err := resolve(p.Spec)
		if err != nil {
			fmt.Fprintf(out, "FAIL %s: %s: could not resolve %s: %v\n", p.File, p.Step, p.Spec, err)
			failures++
			continue
		}
		effective, err := concreteToolchain(p.Toolchain)
		if err != nil {
			fmt.Fprintf(out, "FAIL %s: %s: %s (%s); cannot verify %s\n", p.File, p.Step, err, p.Source, p.Spec)
			failures++
			continue
		}
		if versionAtLeast(effective, normalizeVersion(required)) {
			fmt.Fprintf(out, "ok   %s: %s needs go >= %s, %s provides go %s\n", p.File, p.Spec, normalizeVersion(required), p.Source, effective)
		} else {
			fmt.Fprintf(out, "FAIL %s: %s: %s requires go >= %s but %s provides go %s\n", p.File, p.Step, p.Spec, normalizeVersion(required), p.Source, effective)
			failures++
		}
	}
	if failures != 0 {
		return fmt.Errorf("check-pinned-go-tools: %d of %d pinned tool(s) are not runnable with their step toolchain", failures, len(pins))
	}
	fmt.Fprintf(out, "check-pinned-go-tools: %d pinned tool(s) verified\n", len(pins))
	return nil
}

func workflowFiles(dir string) ([]string, error) {
	var files []string
	for _, ext := range []string{"*.yml", "*.yaml"} {
		m, err := filepath.Glob(filepath.Join(dir, ext))
		if err != nil {
			return nil, err
		}
		files = append(files, m...)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("check-pinned-go-tools: no workflow files (*.yml, *.yaml) in %s", dir)
	}
	return files, nil
}

func scanWorkflow(file string) ([]pin, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	var wf workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	if len(wf.Jobs) == 0 {
		return nil, fmt.Errorf("parse %s: no jobs found", file)
	}
	workflowGTC := stringValue(wf.Env, "GOTOOLCHAIN")
	var result []pin
	for jobName, j := range wf.Jobs {
		jobGTC := stringValue(j.Env, "GOTOOLCHAIN")
		setupVersion, setupSource := "", ""
		for i, s := range j.Steps {
			where := s.Name
			if where == "" {
				where = fmt.Sprintf("%s step %d", jobName, i+1)
			}
			if isSetupGo(s.Uses) {
				v, source, err := setupGoVersion(s.With)
				if err != nil {
					return nil, fmt.Errorf("%s: %s: %w", file, where, err)
				}
				setupVersion, setupSource = v, source
				continue
			}
			if strings.TrimSpace(s.Run) == "" {
				continue
			}
			commands, err := goCommands(s.Run)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", file, where, err)
			}
			for _, c := range commands {
				if c.spec == "" {
					return nil, fmt.Errorf("%s: %s: go %s has no tagged module pin", file, where, c.subcommand)
				}
				if !validModuleVersion(c.spec) {
					return nil, fmt.Errorf("%s: %s: %s is not a valid tagged Go module version", file, where, c.spec)
				}
				gtc, source := c.inlineGTC, "inline GOTOOLCHAIN="+c.inlineGTC
				if gtc == "" {
					gtc, source = stringValue(s.Env, "GOTOOLCHAIN"), "step env GOTOOLCHAIN="+stringValue(s.Env, "GOTOOLCHAIN")
				}
				if gtc == "" {
					gtc, source = jobGTC, "job env GOTOOLCHAIN="+jobGTC
				}
				if gtc == "" {
					gtc, source = workflowGTC, "workflow env GOTOOLCHAIN="+workflowGTC
				}
				if gtc == "" {
					gtc, source = setupVersion, setupSource
				}
				if gtc == "" {
					return nil, fmt.Errorf("%s: %s: %s runs before a deterministic Go toolchain is configured", file, where, c.spec)
				}
				result = append(result, pin{File: file, Job: jobName, Step: where, Spec: c.spec, Toolchain: gtc, Source: source, Line: c.line})
			}
		}
	}
	return result, nil
}

func isSetupGo(uses string) bool {
	parts := strings.SplitN(strings.TrimSpace(uses), "@", 2)
	return len(parts) == 2 && parts[0] == "actions/setup-go" && parts[1] != ""
}

func setupGoVersion(with map[string]any) (string, string, error) {
	if v := stringValue(with, "go-version"); v != "" {
		return v, "setup-go go-version=" + v, nil
	}
	if file := stringValue(with, "go-version-file"); file != "" {
		v, err := readGoDirective(file)
		if err != nil {
			return "", "", fmt.Errorf("setup-go go-version-file=%s: %w", file, err)
		}
		return v, fmt.Sprintf("setup-go go-version-file=%s (go %s)", file, v), nil
	}
	return "", "", errors.New("actions/setup-go must pin go-version or go-version-file")
}

func stringValue(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func readGoDirective(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return goDirective(data)
}

type commandPin struct {
	subcommand, spec, inlineGTC string
	line                        int
}

// Direct-invocation contract
//
// A workflow directly runs a Go tool when `go run module@version` or
// `go install module@version` is reachable as the command word of a simple
// command inside the step's own shell text. That includes every syntactic
// position (command substitutions and backticks, subshells, command groups,
// pipelines, if/while/until conditions, functions and their bodies, prefix and
// suffix redirections), leading shell assignments, arbitrary line breaks and
// continuations, Go's own `-C dir` flag in front of the subcommand, and the
// transparent wrappers `command`, `exec` and `env` (whose own option grammars
// are modelled, including `--`, `-a`, `-u`, and the argv that `env -S`
// re-splits). Launchers (nice, nohup, setsid, stdbuf, timeout, ionice, chrt,
// sudo, doas, su, runuser, xargs, parallel, flock, watch, ssh, busybox,
// systemd-run, find, script, expect) and child interpreters (`sh -c '...'`)
// all delegate their command word back into the same classification path, so a
// delegated command word is classified exactly like a top-level one. Static
// text the job's own shell hands to `eval`, to a `-c`-style launcher option
// (`su -c`, `flock -c`, `script -c`, `expect -c`) or to `trap` is scanned
// recursively, because it executes in the same job.
//
// Everything the guard cannot resolve statically fails closed instead of being
// silently skipped: dynamic command words followed by run/install, dynamic
// operands in front of a Go invocation (a launcher operand included), dynamic
// GOTOOLCHAIN assignments, launcher arguments that still spell a Go invocation
// the guard could not attribute to a command word, text handed to a command
// word this guard does not model (trap, script, expect, or any other local
// executor), interpreters reading a script file, unknown wrapper flags, unknown
// Go flags, and malformed shell. Only the command words listed in nonExecutors
// have their arguments treated as inert data; every other command word is
// checked with hiddenGoInvocation, and at(1)/batch(1) fail closed outright
// because the script they run arrives on standard input.
func goCommands(src string) ([]commandPin, error) {
	return scanShell(src, 0)
}

func scanShell(src string, depth int) ([]commandPin, error) {
	parsedSource, err := shellWithGitHubExpressions(src)
	if err != nil {
		return nil, err
	}
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(parsedSource), "workflow run")
	if err != nil {
		return nil, fmt.Errorf("unterminated or invalid shell syntax: %w", err)
	}
	var result []commandPin
	var walkErr error
	syntax.Walk(file, func(node syntax.Node) bool {
		if walkErr != nil {
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		pins, err := directGoPins(call, depth)
		if err != nil {
			walkErr = err
			return false
		}
		result = append(result, pins...)
		return true
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return result, nil
}

// GitHub expands ${{ ... }} before invoking the configured shell. Replace each
// expression with a dynamic shell parameter so the Bash parser sees the same
// runtime boundary; direct Go arguments containing one then fail closed as
// non-static. Newlines are retained so diagnostic line numbers remain stable.
func shellWithGitHubExpressions(src string) (string, error) {
	var out strings.Builder
	for {
		start := strings.Index(src, "${{")
		if start < 0 {
			out.WriteString(src)
			return out.String(), nil
		}
		out.WriteString(src[:start])
		rest := src[start+3:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return "", errors.New("unterminated GitHub Actions expression")
		}
		out.WriteString("${GITHUB_ACTIONS_EXPR}")
		for _, r := range rest[:end] {
			if r == '\n' {
				out.WriteByte('\n')
			}
		}
		src = rest[end+2:]
	}
}

func directGoPins(call *syntax.CallExpr, depth int) ([]commandPin, error) {
	if len(call.Args) == 0 {
		return nil, nil
	}
	line := int(call.Pos().Line())
	inline := ""
	for _, a := range call.Assigns {
		if a.Name == nil || a.Name.Value != "GOTOOLCHAIN" || a.Value == nil {
			continue
		}
		value, ok := staticWord(a.Value)
		if !ok {
			return nil, fmt.Errorf("line %d: dynamic GOTOOLCHAIN assignment cannot be verified", line)
		}
		inline = value
	}
	return classifyCommand(callArgs(call), 0, line, inline, depth)
}

// shellArg is one word of a simple command together with the static value the
// shell would pass for it. static is false when the word holds an expansion or
// substitution, so its runtime value cannot be known here.
type shellArg struct {
	val    string
	static bool
	line   int
}

func callArgs(call *syntax.CallExpr) []shellArg {
	args := make([]shellArg, 0, len(call.Args))
	for _, w := range call.Args {
		value, ok := staticWord(w)
		args = append(args, shellArg{val: value, static: ok, line: int(w.Pos().Line())})
	}
	return args
}

// staticArgs rebuilds the argv env produces by re-splitting a `-S` value.
func staticArgs(line int, values ...string) []shellArg {
	args := make([]shellArg, 0, len(values))
	for _, v := range values {
		args = append(args, shellArg{val: v, static: true, line: line})
	}
	return args
}

func staticArg(words []shellArg, i, line int) (string, error) {
	if i >= len(words) {
		return "", io.EOF
	}
	if !words[i].static {
		return "", fmt.Errorf("line %d: dynamic shell word cannot be verified", line)
	}
	return words[i].val, nil
}

// classifyCommand resolves the command word at words[i] and returns every Go
// pin that word directly runs. Every wrapper (command, exec, env), every
// launcher this guard models and every child interpreter delegates back into
// this function, so a delegated command word is classified through exactly the
// same path as a top-level one.
func classifyCommand(words []shellArg, i, line int, inline string, depth int) ([]commandPin, error) {
	if i >= len(words) {
		return nil, nil
	}
	if !words[i].static {
		// The command word itself is not static, so it cannot be classified.
		// Refuse instead of skipping when a run/install invocation could follow.
		if containsGoInvocation(words, i+1) || containsGoSubcommand(words, i+1) {
			return nil, fmt.Errorf("line %d: dynamic command name cannot be verified", line)
		}
		return nil, nil
	}
	switch first := filepath.Base(words[i].val); first {
	case "go":
		return goPins(words, i, line, inline)
	case "command":
		j := i + 1
		for j < len(words) {
			arg, err := staticArg(words, j, line)
			if err != nil {
				return nil, err
			}
			if arg == "--" {
				j++
				break
			}
			if arg == "-p" || arg == "-v" || arg == "-V" {
				j++
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return nil, fmt.Errorf("line %d: unsupported command wrapper flag %s", line, arg)
			}
			break
		}
		return classifyCommand(words, j, line, inline, depth)
	case "exec":
		j := i + 1
		for j < len(words) {
			arg, err := staticArg(words, j, line)
			if err != nil {
				return nil, err
			}
			if arg == "--" {
				j++
				break
			}
			if arg == "-a" || arg == "--argv0" {
				if _, err := staticArg(words, j+1, line); err != nil {
					return nil, fmt.Errorf("line %d: exec %s requires a value", line, arg)
				}
				j += 2
				continue
			}
			if strings.HasPrefix(arg, "-") && strings.Trim(arg[1:], "cl") == "" {
				j++
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return nil, fmt.Errorf("line %d: unsupported exec wrapper flag %s", line, arg)
			}
			break
		}
		return classifyCommand(words, j, line, inline, depth)
	case "env":
		return envPins(words, i+1, line, inline, depth)
	case "eval":
		// eval executes its static text in this same shell, so it is
		// scanned recursively; dynamic eval text fails closed.
		var text []string
		for j := i + 1; j < len(words); j++ {
			arg, err := staticArg(words, j, line)
			if err != nil {
				return nil, fmt.Errorf("line %d: eval argument cannot be statically verified", line)
			}
			text = append(text, arg)
		}
		return scanStaticShellText(strings.Join(text, " "), line, inline, depth)
	case "sh", "bash", "zsh", "dash", "ksh", "ash":
		return directGoPinsInShell(first, words, i+1, line, inline, depth)
	case "trap":
		return trapPins(words, i+1, line, inline, depth)
	default:
		if grammar, ok := launchers[first]; ok {
			return launcherPins(first, grammar, words, i+1, line, inline, depth)
		}
		if stdinExecutors[first] {
			// at(1)/batch(1) run the shell text they read from standard input,
			// which this guard cannot see, so they fail closed instead of
			// being silently skipped.
			return nil, fmt.Errorf("line %d: %s reads the script it runs from standard input, which cannot be verified statically", line, first)
		}
		if nonExecutors[first] {
			return nil, nil
		}
		// The command word is not modelled: it may be a local executor
		// (trap, script, expect, ...) this guard does not know, so text it is
		// handed is never silently skipped.
		if err := hiddenGoInvocation(words, i+1, line, first); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

// nonExecutors are the command words whose arguments are inert data rather than
// shell text or a command word the shell would run, so a Go invocation spelled
// in their argv cannot execute. Leaving a command out of this list can only add
// blocking (the default arm then fails closed on it), never hide a pin, so the
// list stays narrow: anything that runs a command word it is handed - git
// (bisect run), make, docker, kubectl, xargs, find, ssh, python, node, npm
// exec, ... - is deliberately absent.
var nonExecutors = map[string]bool{
	"actionlint": true, "awk": true, "basename": true, "cat": true,
	"chmod": true, "chown": true, "cp": true, "curl": true, "cut": true,
	"date": true, "dirname": true, "echo": true, "false": true, "file": true,
	"gofmt": true, "golangci-lint": true, "grep": true, "gzip": true,
	"head": true, "hostname": true, "id": true, "jq": true, "ln": true,
	"ls": true, "mkdir": true, "mv": true, "printenv": true, "printf": true,
	"pwd": true, "readlink": true, "realpath": true, "rm": true, "sed": true,
	"seq": true, "shellcheck": true, "sleep": true, "sort": true,
	"stat": true, "tail": true, "tar": true, "tee": true, "test": true,
	"touch": true, "tr": true, "true": true, "uname": true, "uniq": true,
	"unzip": true, "wc": true, "wget": true, "which": true, "whoami": true,
	"yq": true, "zip": true, "[": true,
}

// stdinExecutors run shell text they read from standard input.
var stdinExecutors = map[string]bool{"at": true, "batch": true}

// trapPins handles the Bash trap builtin. Its first operand is the shell text
// the job's own shell runs later (the remaining operands are signal specs), so
// that text is scanned recursively; the query forms (-l, -p) run no text, a
// dynamic action cannot be verified and fails closed, and the whole builtin is
// re-checked for a Go invocation the guard could not attribute to a command
// word.
func trapPins(words []shellArg, start, line int, inline string, depth int) ([]commandPin, error) {
	if start >= len(words) {
		return nil, nil
	}
	if words[start].static {
		switch words[start].val {
		case "-l", "-p", "--help", "--print":
			// Query forms: they list or print traps instead of running text.
			return nil, nil
		}
	}
	action, err := staticArg(words, start, line)
	if err != nil {
		return nil, fmt.Errorf("line %d: trap argument cannot be statically verified", line)
	}
	var pins []commandPin
	if action != "-" {
		scanned, err := scanStaticShellText(action, line, inline, depth)
		if err != nil {
			return nil, err
		}
		pins = append(pins, scanned...)
	}
	if err := hiddenGoInvocation(words, start, line, "trap"); err != nil {
		return nil, err
	}
	return pins, nil
}

// envPins models env(1)'s own option grammar. env consumes NAME=VALUE
// assignments both before and after `--`, re-splits `-S`/`--split-string`
// values into argv (GNU env does), and then executes the word that remains -
// which is classified through the shared command-word path.
func envPins(words []shellArg, start, line int, inline string, depth int) ([]commandPin, error) {
	endOfOptions := false
	for j := start; j < len(words); j++ {
		arg, err := staticArg(words, j, line)
		if err != nil {
			return nil, err
		}
		if !endOfOptions {
			if arg == "--" {
				endOfOptions = true
				continue
			}
			if name, value, ok := splitAssignment(arg); ok {
				if name == "GOTOOLCHAIN" {
					inline = value
				}
				continue
			}
			if strings.HasPrefix(arg, "-") && arg != "-" {
				name, attached, hasValue := splitFlag(arg)
				switch name {
				case "-u", "--unset", "-C", "--chdir":
					if !hasValue {
						if _, err := staticArg(words, j+1, line); err != nil {
							if errors.Is(err, io.EOF) {
								return nil, fmt.Errorf("line %d: %s requires a value", line, name)
							}
							return nil, err
						}
						j++
					}
					continue
				case "-S", "--split-string":
					value := attached
					if !hasValue {
						next, err := staticArg(words, j+1, line)
						if err != nil {
							if errors.Is(err, io.EOF) {
								return nil, fmt.Errorf("line %d: %s requires a value", line, name)
							}
							return nil, err
						}
						value = next
						j++
					}
					split, err := splitEnvString(value)
					if err != nil {
						return nil, fmt.Errorf("line %d: env %s: %w", line, name, err)
					}
					expanded := append(staticArgs(line, split...), words[j+1:]...)
					return envPins(expanded, 0, line, inline, depth)
				case "-i", "--ignore-environment", "-0", "--null", "--debug", "--help", "--version":
					continue
				default:
					return nil, fmt.Errorf("line %d: unsupported env wrapper flag %s", line, name)
				}
			}
		} else if name, value, ok := splitAssignment(arg); ok {
			// env keeps parsing NAME=VALUE assignments after `--`.
			if name == "GOTOOLCHAIN" {
				inline = value
			}
			continue
		}
		return classifyCommand(words, j, line, inline, depth)
	}
	return nil, nil
}

// launcherGrammar models the words a launcher consumes between itself and the
// command it executes, so that command can be classified through the same path
// as a top-level one. Flags the table does not list are skipped, which can only
// move the command word later, never hide it: hiddenGoInvocation refuses
// anything left over that spells a Go invocation. A bare `--` ends the option
// list without taking a value (only a two-character flag with a non-empty
// value-flags letter consumes the following word).
type launcherGrammar struct {
	flags      string // short options taking no value
	valueFlags string // short options taking a value (separate or attached)
	execFlags  string // short options whose value is shell text a child shell runs
	operands   int    // leading operands before the command word
	findExec   bool   // find: command words follow -exec/-execdir/-ok/-okdir
}

var launchers = map[string]launcherGrammar{
	"nice":        {valueFlags: "n"},
	"nohup":       {},
	"setsid":      {flags: "cfw"},
	"stdbuf":      {valueFlags: "ioe"},
	"timeout":     {flags: "v", valueFlags: "ks", operands: 1},
	"ionice":      {flags: "t", valueFlags: "cnpPu"},
	"chrt":        {flags: "abdfimoprRv", valueFlags: "DTP", operands: 1},
	"sudo":        {flags: "AbBEHikKnPSvVles", valueFlags: "CDghprtTuU"},
	"doas":        {flags: "nsLd", valueFlags: "uCa"},
	"su":          {flags: "flmp", valueFlags: "gGsw", execFlags: "c"},
	"runuser":     {flags: "flmp", valueFlags: "gGsw", execFlags: "c"},
	"xargs":       {flags: "tporx0", valueFlags: "adEILnPs"},
	"parallel":    {flags: "k", valueFlags: "jNnS"},
	"flock":       {flags: "Fnoxsv", valueFlags: "Ew", execFlags: "c", operands: 1},
	"watch":       {flags: "cdegptbw", valueFlags: "nq"},
	"ssh":         {flags: "46AaCfGgKkMNnqsTtVvXxYy", valueFlags: "bcDEeFIiJLlmOopQRSWw", operands: 1},
	"busybox":     {},
	"systemd-run": {flags: "qG", valueFlags: "pEPt"},
	"script":      {flags: "aefq", valueFlags: "t", execFlags: "c"},
	"expect":      {flags: "dfi", valueFlags: "D", execFlags: "c"},
	"find":        {findExec: true},
}

func launcherPins(name string, grammar launcherGrammar, words []shellArg, start, line int, inline string, depth int) ([]commandPin, error) {
	if grammar.findExec {
		return findPins(name, words, start, line, inline, depth)
	}
	// A launcher option can carry the command as shell text a child shell runs
	// (su -c '...', flock -c '...'); that text executes in the same job.
	var pins []commandPin
	for j := start; j < len(words); j++ {
		if !words[j].static {
			continue
		}
		flag, attached, hasValue := splitFlag(words[j].val)
		if grammar.execFlags == "" || !strings.HasPrefix(flag, "-") || !strings.Contains(grammar.execFlags, strings.TrimLeft(flag, "-")) {
			continue
		}
		text := attached
		if !hasValue {
			if j+1 >= len(words) || !words[j+1].static {
				return nil, fmt.Errorf("line %d: %s %s command cannot be statically verified", line, name, words[j].val)
			}
			text = words[j+1].val
		}
		scanned, err := scanStaticShellText(text, line, inline, depth)
		if err != nil {
			return nil, err
		}
		pins = append(pins, scanned...)
	}

	// Then the positional command word.
	j, skipped := start, 0
	for j < len(words) {
		if !words[j].static {
			if skipped < grammar.operands {
				// A dynamic operand cannot be classified, but the word this
				// launcher actually executes is the one after the modelled
				// operands, so the search continues there.
				skipped++
				j++
				continue
			}
			classified, err := classifyCommand(words, j, line, inline, depth)
			if err != nil {
				return nil, err
			}
			return append(pins, classified...), nil
		}
		arg := words[j].val
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flag, _, hasValue := splitFlag(arg)
			short := strings.TrimLeft(flag, "-")
			if len(flag) == 2 && short != "" && strings.Contains(grammar.valueFlags, short) {
				if !hasValue {
					if j+1 >= len(words) {
						return nil, fmt.Errorf("line %d: %s %s requires a value", line, name, flag)
					}
					if _, err := staticArg(words, j+1, line); err != nil {
						return nil, err
					}
					j++
				}
			}
			j++
			continue
		}
		if skipped < grammar.operands {
			skipped++
			j++
			continue
		}
		classified, err := classifyCommand(words, j, line, inline, depth)
		if err != nil {
			return nil, err
		}
		if len(classified) == 0 {
			if err := hiddenGoInvocation(words, start, line, name); err != nil {
				return nil, err
			}
		}
		return append(pins, classified...), nil
	}
	return pins, nil
}

// findPins handles find(1), whose actions execute a command word of their own.
func findPins(name string, words []shellArg, start, line int, inline string, depth int) ([]commandPin, error) {
	var pins []commandPin
	found := false
	for j := start; j < len(words); j++ {
		if !words[j].static {
			continue
		}
		switch words[j].val {
		case "-exec", "-execdir", "-ok", "-okdir":
			classified, err := classifyCommand(words, j+1, line, inline, depth)
			if err != nil {
				return nil, err
			}
			pins = append(pins, classified...)
			found = true
		}
	}
	if !found {
		if err := hiddenGoInvocation(words, start, line, name); err != nil {
			return nil, err
		}
	}
	return pins, nil
}

// hiddenGoInvocation refuses a Go invocation the guard could not attribute to a
// command word: a static `go run|install` pair, or a dynamic word a following
// run/install subcommand could belong to.
func hiddenGoInvocation(words []shellArg, start, line int, context string) error {
	if containsGoInvocation(words, start) || dynamicRunInstall(words, start) {
		return fmt.Errorf("line %d: %s launches a Go tool invocation that cannot be verified statically", line, context)
	}
	return nil
}

func dynamicRunInstall(words []shellArg, start int) bool {
	for i := start; i+1 < len(words); i++ {
		if words[i].static {
			continue
		}
		if words[i+1].static && (words[i+1].val == "run" || words[i+1].val == "install") {
			return true
		}
	}
	return false
}

// goPins classifies `go run|install` at words[goIndex]. `go -C dir` is the only
// Go flag legal before the subcommand, so it is consumed first.
func goPins(words []shellArg, goIndex, line int, inline string) ([]commandPin, error) {
	i := goIndex + 1
	for i < len(words) && words[i].static && (words[i].val == "-C" || strings.HasPrefix(words[i].val, "-C=")) {
		if words[i].val == "-C" {
			if _, err := staticArg(words, i+1, line); err != nil {
				return nil, fmt.Errorf("line %d: go -C requires a directory", line)
			}
			i += 2
			continue
		}
		i++
	}
	if i >= len(words) {
		return nil, nil
	}
	sub, err := staticArg(words, i, line)
	if err != nil {
		return nil, err
	}
	if sub != "run" && sub != "install" {
		return nil, nil
	}
	i++
	for i < len(words) {
		arg, err := staticArg(words, i, line)
		if err != nil {
			return nil, err
		}
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		name := arg
		if at := strings.IndexByte(name, '='); at >= 0 {
			name = name[:at]
		}
		needsValue, known := goFlagKind(name, sub)
		if !known {
			return nil, fmt.Errorf("line %d: unsupported go %s flag %s", line, sub, name)
		}
		i++
		if needsValue && !strings.Contains(arg, "=") {
			if i >= len(words) {
				return nil, fmt.Errorf("line %d: %s requires a value", line, name)
			}
			if _, err := staticArg(words, i, line); err != nil {
				return nil, err
			}
			i++
		}
	}
	if i >= len(words) {
		return nil, fmt.Errorf("line %d: go %s has no package argument", line, sub)
	}

	makePin := func(spec string) commandPin {
		return commandPin{subcommand: sub, spec: spec, inlineGTC: inline, line: words[goIndex].line}
	}
	if sub == "run" {
		spec, err := staticArg(words, i, line)
		if err != nil {
			return nil, err
		}
		if localGoTarget(spec) {
			return nil, nil
		}
		if !strings.Contains(spec, "@") {
			return []commandPin{makePin("")}, nil
		}
		return []commandPin{makePin(spec)}, nil
	}

	var pins []commandPin
	for ; i < len(words); i++ {
		spec, err := staticArg(words, i, line)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(spec, "-") {
			return nil, fmt.Errorf("line %d: go install flag %s appears after a package", line, spec)
		}
		if localGoTarget(spec) {
			continue
		}
		if !strings.Contains(spec, "@") {
			pins = append(pins, makePin(""))
			continue
		}
		pins = append(pins, makePin(spec))
	}
	return pins, nil
}

// directGoPinsInShell handles a child interpreter (`sh -c '...'`). The script
// text executes in the job, so a static script is scanned recursively; a
// non-static argument or a script file cannot be verified and fails closed.
func directGoPinsInShell(interp string, words []shellArg, start, line int, inline string, depth int) ([]commandPin, error) {
	script := ""
	haveScript := false
	for j := start; j < len(words); j++ {
		if !words[j].static {
			return nil, fmt.Errorf("line %d: %s argument cannot be statically verified", line, interp)
		}
		arg := words[j].val
		switch {
		case arg == "-c" || arg == "--command":
			if j+1 >= len(words) {
				return nil, fmt.Errorf("line %d: %s %s requires a script", line, interp, arg)
			}
			if !words[j+1].static {
				return nil, fmt.Errorf("line %d: %s %s script cannot be statically verified", line, interp, arg)
			}
			script, haveScript = words[j+1].val, true
			j++
		case strings.HasPrefix(arg, "-"):
			// Other interpreter options do not change the script text.
		case !haveScript:
			return nil, fmt.Errorf("line %d: %s script file %s cannot be statically verified", line, interp, arg)
		}
	}
	if !haveScript {
		return nil, nil
	}
	return scanStaticShellText(script, line, inline, depth)
}

// scanStaticShellText scans shell text the job's own shell executes (eval or
// `sh -c '...'`). The recursion is depth-bounded and the surrounding inline
// GOTOOLCHAIN assignment flows into the pins found inside the text.
func scanStaticShellText(src string, line int, inlineGTC string, depth int) ([]commandPin, error) {
	if depth >= 8 {
		return nil, fmt.Errorf("line %d: nested shell text is nested too deeply to verify", line)
	}
	pins, err := scanShell(src, depth+1)
	if err != nil {
		return nil, err
	}
	for i := range pins {
		if pins[i].inlineGTC == "" {
			pins[i].inlineGTC = inlineGTC
		}
	}
	return pins, nil
}

// containsGoInvocation reports whether the static arguments from start spell a
// `go run` or `go install` invocation.
func containsGoInvocation(words []shellArg, start int) bool {
	for i := start; i+1 < len(words); i++ {
		if !words[i].static || filepath.Base(words[i].val) != "go" {
			continue
		}
		if words[i+1].static && (words[i+1].val == "run" || words[i+1].val == "install") {
			return true
		}
	}
	return false
}

// containsGoSubcommand reports whether an unclassified command carries a
// run/install subcommand operand, which is what a hidden dynamic `go` would use.
func containsGoSubcommand(words []shellArg, start int) bool {
	for i := start; i < len(words); i++ {
		if words[i].static && (words[i].val == "run" || words[i].val == "install") {
			return true
		}
	}
	return false
}

// splitFlag splits `-x`, `-xVALUE`, `--name` and `--name=VALUE`.
func splitFlag(arg string) (name, value string, hasValue bool) {
	if strings.HasPrefix(arg, "--") {
		if at := strings.IndexByte(arg, '='); at >= 0 {
			return arg[:at], arg[at+1:], true
		}
		return arg, "", false
	}
	if len(arg) > 2 {
		return arg[:2], arg[2:], true
	}
	return arg, "", false
}

// splitEnvString re-splits an `env -S`/`--split-string` value the way GNU env
// does before it invokes the command: whitespace separates words, quotes group
// them, and a backslash escapes the following character.
func splitEnvString(value string) ([]string, error) {
	var words []string
	var b strings.Builder
	started := false
	var quote byte
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
				continue
			}
			if c == '\\' && quote == '"' && i+1 < len(value) && (value[i+1] == '"' || value[i+1] == '\\') {
				i++
				b.WriteByte(value[i])
				continue
			}
			b.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			started = true
		case c == '\\':
			if i+1 >= len(value) {
				return nil, errors.New("trailing backslash")
			}
			i++
			b.WriteByte(value[i])
			started = true
		case c == ' ' || c == '\t' || c == '\n':
			if started {
				words = append(words, b.String())
				b.Reset()
				started = false
			}
		default:
			b.WriteByte(c)
			started = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if started {
		words = append(words, b.String())
	}
	return words, nil
}

// unescapeLit resolves backslash escapes in an unquoted word part so the value
// the shell actually runs is classified (`\go run` is `go run`).
func unescapeLit(value string) string {
	if !strings.Contains(value, "\\") {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
		}
		b.WriteByte(value[i])
	}
	return b.String()
}

func staticWord(word *syntax.Word) (string, bool) {
	var b strings.Builder
	var addParts func([]syntax.WordPart) bool
	addParts = func(parts []syntax.WordPart) bool {
		for _, part := range parts {
			switch x := part.(type) {
			case *syntax.Lit:
				b.WriteString(unescapeLit(x.Value))
			case *syntax.SglQuoted:
				b.WriteString(x.Value)
			case *syntax.DblQuoted:
				if !addParts(x.Parts) {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	if !addParts(word.Parts) {
		return "", false
	}
	return b.String(), true
}

func splitAssignment(word string) (string, string, bool) {
	at := strings.IndexByte(word, '=')
	if at <= 0 {
		return "", "", false
	}
	name := word[:at]
	for i, r := range name {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return "", "", false
		}
	}
	return name, word[at+1:], true
}

func localGoTarget(arg string) bool {
	return strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "/") || strings.HasSuffix(arg, ".go")
}

func goFlagKind(flag, subcommand string) (needsValue, known bool) {
	switch flag {
	case "-C", "-asmflags", "-buildmode", "-compiler", "-covermode", "-coverpkg", "-gccgoflags", "-gcflags", "-installsuffix", "-ldflags", "-mod", "-modfile", "-overlay", "-p", "-pgo", "-pkgdir", "-tags", "-toolexec":
		return true, true
	case "-a", "-asan", "-buildvcs", "-cover", "-linkshared", "-modcacherw", "-msan", "-n", "-race", "-trimpath", "-v", "-work", "-x":
		return false, true
	case "-exec":
		return subcommand == "run", subcommand == "run"
	default:
		return false, false
	}
}

func validModuleVersion(spec string) bool {
	at := strings.LastIndexByte(spec, '@')
	return at > 0 && semver.IsValid(spec[at+1:])
}

func concreteToolchain(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "auto" || v == "local" {
		return "", fmt.Errorf("GOTOOLCHAIN=%s is not an exact toolchain", v)
	}
	if !concreteGo.MatchString(v) {
		return "", fmt.Errorf("toolchain %q is not a concrete version", v)
	}
	return normalizeVersion(v), nil
}

func normalizeVersion(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "go")
	v = strings.TrimSuffix(v, ".x") + ".0"
	parts := strings.Split(v, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return strings.Join(parts[:3], ".")
}

func versionAtLeast(have, need string) bool {
	a, b := versionParts(have), versionParts(need)
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

func versionParts(v string) [3]int {
	var p [3]int
	fmt.Sscanf(normalizeVersion(v), "%d.%d.%d", &p[0], &p[1], &p[2])
	return p
}

func parseRequirementOverride(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	m := map[string]string{}
	for _, item := range strings.Split(raw, ";") {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

var moduleProxy = "https://proxy.golang.org"

func resolveModuleRequirement(spec string) (string, error) {
	at := strings.LastIndex(spec, "@")
	if at < 1 {
		return "", errors.New("missing version")
	}
	path, version := spec[:at], spec[at+1:]
	client := &http.Client{Timeout: 30 * time.Second}
	for {
		u := moduleProxy + "/" + escapeModulePath(path) + "/@v/" + url.PathEscape(version) + ".mod"
		resp, err := client.Get(u)
		if err != nil {
			return "", fmt.Errorf("module proxy request: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		closeErr := resp.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read module proxy response: %w", readErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close module proxy response: %w", closeErr)
		}
		if resp.StatusCode == http.StatusOK {
			return goDirective(data)
		}
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusGone {
			return "", fmt.Errorf("module proxy returned HTTP %d", resp.StatusCode)
		}
		i := strings.LastIndex(path, "/")
		if i <= 0 {
			break
		}
		path = path[:i]
	}
	return "", errors.New("module/version could not be resolved")
}

func escapeModulePath(path string) string {
	var b strings.Builder
	for _, r := range path {
		switch {
		case r == '!':
			b.WriteString("!!")
		case r >= 'A' && r <= 'Z':
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func goDirective(data []byte) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1], nil
		}
	}
	return "", errors.New("no go directive")
}
