package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.minekube.com/gate/internal/wasm/analyze"
	"go.minekube.com/gate/internal/wasm/generate"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = io.WriteString(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (args[0] != "generate" && args[0] != "check") {
		return usageError()
	}
	command := args[0]
	flags := flag.NewFlagSet("gate-wasm-gen "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", "", "Gate repository root (defaults to nearest go.mod)")
	output := flags.String("out", "", "generated artifact directory")
	nativeOutput := flags.String(
		"native-out",
		"",
		"generated Rust host source directory",
	)
	publicOutput := flags.String(
		"public-out",
		"",
		"public language-neutral contract directory",
	)
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return usageError()
	}

	var err error
	if *repo == "" {
		*repo, err = findRepository()
		if err != nil {
			return err
		}
	} else {
		*repo, err = filepath.Abs(*repo)
		if err != nil {
			return fmt.Errorf("resolve repository path: %w", err)
		}
	}
	if *output == "" {
		*output = filepath.Join(*repo, "internal", "wasm", "api")
	} else {
		*output, err = filepath.Abs(*output)
		if err != nil {
			return fmt.Errorf("resolve output path: %w", err)
		}
	}
	if *nativeOutput != "" {
		*nativeOutput, err = filepath.Abs(*nativeOutput)
		if err != nil {
			return fmt.Errorf("resolve native output path: %w", err)
		}
	}
	if *publicOutput != "" {
		*publicOutput, err = filepath.Abs(*publicOutput)
		if err != nil {
			return fmt.Errorf("resolve public output path: %w", err)
		}
	}
	if _, err := os.Stat(filepath.Join(*repo, "go.mod")); err != nil {
		return fmt.Errorf("%s is not a Gate repository root: %w", *repo, err)
	}

	api, err := analyze.Analyze(context.Background(), analyze.GateOptions(*repo))
	if err != nil {
		return fmt.Errorf("analyze Gate API: %w", err)
	}
	artifacts, err := generate.Artifacts(api)
	if err != nil {
		return err
	}
	var nativeArtifacts map[string][]byte
	if *nativeOutput != "" {
		nativeArtifacts, err = generate.NativeArtifacts(api)
		if err != nil {
			return err
		}
	}
	var publicArtifacts map[string][]byte
	if *publicOutput != "" {
		publicArtifacts, err = generate.PublicArtifacts(api)
		if err != nil {
			return err
		}
	}

	switch command {
	case "generate":
		if err := writeArtifacts(*output, artifacts); err != nil {
			return err
		}
		if *nativeOutput != "" {
			if err := writeArtifacts(*nativeOutput, nativeArtifacts); err != nil {
				return err
			}
		}
		if *publicOutput != "" {
			if err := writeArtifacts(*publicOutput, publicArtifacts); err != nil {
				return err
			}
		}
		_, err = fmt.Fprintf(
			stdout,
			"generated %d WebAssembly API artifacts in %s\n",
			len(artifacts)+len(nativeArtifacts)+len(publicArtifacts),
			*output,
		)
		return err
	case "check":
		if err := checkArtifacts(*output, artifacts); err != nil {
			return err
		}
		if *nativeOutput != "" {
			if err := checkArtifacts(*nativeOutput, nativeArtifacts); err != nil {
				return err
			}
		}
		if *publicOutput != "" {
			if err := checkArtifacts(*publicOutput, publicArtifacts); err != nil {
				return err
			}
		}
		_, err = fmt.Fprintln(stdout, "WebAssembly API artifacts are current")
		return err
	default:
		panic("command validation drift")
	}
}

func usageError() error {
	return fmt.Errorf(
		"usage: gate-wasm-gen <generate|check> [-repo path] [-out path] [-native-out path] [-public-out path]",
	)
}

func findRepository() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("could not find a repository go.mod")
		}
		current = parent
	}
}

func writeArtifacts(output string, artifacts map[string][]byte) error {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return fmt.Errorf("create artifact directory %s: %w", output, err)
	}
	transaction, err := os.MkdirTemp(
		filepath.Dir(output),
		".gate-wasm-gen-*",
	)
	if err != nil {
		return fmt.Errorf("create generation transaction: %w", err)
	}
	defer os.RemoveAll(transaction)
	newDirectory := filepath.Join(transaction, "new")
	backupDirectory := filepath.Join(transaction, "backup")
	if err := os.MkdirAll(newDirectory, 0o755); err != nil {
		return fmt.Errorf("create generated staging directory: %w", err)
	}
	if err := os.MkdirAll(backupDirectory, 0o755); err != nil {
		return fmt.Errorf("create generated backup directory: %w", err)
	}
	names := artifactNames(artifacts)
	for _, name := range names {
		if filepath.Base(name) != name {
			return fmt.Errorf("invalid generated artifact name %q", name)
		}
		if err := os.WriteFile(
			filepath.Join(newDirectory, name),
			artifacts[name],
			0o644,
		); err != nil {
			return fmt.Errorf("stage generated artifact %s: %w", name, err)
		}
	}

	type replacement struct {
		name      string
		hadBackup bool
		installed bool
	}
	var replacements []replacement
	rollback := func() {
		for index := len(replacements) - 1; index >= 0; index-- {
			replacement := replacements[index]
			target := filepath.Join(output, replacement.name)
			if replacement.installed {
				_ = os.Remove(target)
			}
			if replacement.hadBackup {
				_ = os.Rename(
					filepath.Join(backupDirectory, replacement.name),
					target,
				)
			}
		}
	}
	for _, name := range names {
		target := filepath.Join(output, name)
		replacement := replacement{name: name}
		if _, err := os.Stat(target); err == nil {
			if err := os.Rename(
				target,
				filepath.Join(backupDirectory, name),
			); err != nil {
				rollback()
				return fmt.Errorf("back up generated artifact %s: %w", name, err)
			}
			replacement.hadBackup = true
		} else if !os.IsNotExist(err) {
			rollback()
			return fmt.Errorf("inspect generated artifact %s: %w", name, err)
		}
		replacements = append(replacements, replacement)
		if err := os.Rename(filepath.Join(newDirectory, name), target); err != nil {
			rollback()
			return fmt.Errorf("install generated artifact %s: %w", name, err)
		}
		replacements[len(replacements)-1].installed = true
	}
	return nil
}

func checkArtifacts(output string, artifacts map[string][]byte) error {
	var differences []string
	for _, name := range artifactNames(artifacts) {
		actual, err := os.ReadFile(filepath.Join(output, name))
		if os.IsNotExist(err) {
			differences = append(differences, name+" is missing")
			continue
		}
		if err != nil {
			return fmt.Errorf("read generated artifact %s: %w", name, err)
		}
		if bytes.Equal(actual, artifacts[name]) {
			continue
		}
		differences = append(
			differences,
			fmt.Sprintf(
				"%s differs at line %d",
				name,
				firstDifferentLine(actual, artifacts[name]),
			),
		)
	}
	if len(differences) > 0 {
		return fmt.Errorf(
			"WebAssembly API artifacts are stale: %s",
			strings.Join(differences, "; "),
		)
	}
	return nil
}

func artifactNames(artifacts map[string][]byte) []string {
	names := make([]string, 0, len(artifacts))
	for name := range artifacts {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func firstDifferentLine(left, right []byte) int {
	leftLines := bytes.Split(left, []byte{'\n'})
	rightLines := bytes.Split(right, []byte{'\n'})
	limit := min(len(leftLines), len(rightLines))
	for index := range limit {
		if !bytes.Equal(leftLines[index], rightLines[index]) {
			return index + 1
		}
	}
	return limit + 1
}
