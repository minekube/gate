package main

import (
	"strings"
	"testing"
)

// Guards over the operator-facing Geyser configuration guidance: the manual-setup
// comment in the shipped config template (pkg/configs/config.yml) and the manual
// setup / troubleshooting snippets in .web/docs/guide/bedrock.md.
//
// WHAT THESE TESTS PROVE
//
//   - Both surfaces tell an external-Geyser operator to set the HAProxy toggle at the
//     path Geyser 2.x actually reads: advanced.java.use-haproxy-protocol
//     (GeyserConfig.AdvancedJavaConfig#useHaproxyProtocol). Geyser 2.0 sectioned its
//     config; a bare/root "use-proxy-protocol" is not read from that layout.
//   - The docs' YAML snippets nest the toggle under advanced -> java. The check resolves
//     each fenced block's indentation into a dotted path instead of matching a string, so
//     a snippet that keeps the right key at the wrong level still fails.
//   - No copyable snippet teaches a legacy proxy-protocol key. The pre-2.0 key stays
//     documented in prose (as the shape Geyser's own migration rewrites), but pasting it
//     into an already-migrated config is exactly the silent failure this guard exists for.
//   - The pre-2.0 path and the config-version boundary stay written down, because
//     "set advanced.java.use-haproxy-protocol" is only true for Geyser config-version 5
//     and newer: ConfigMigrations.addVersion(5) converts remote.use-proxy-protocol, and a
//     config already at the current version is not migrated again.
//
// WHAT THEY DO NOT PROVE
//
//   - That Geyser reads the key. That was measured against a real Geyser 2.12.0 binary,
//     not by a string match: the manual-setup snippet placed on a config-version 11
//     config logs "Geyser is configured to use proxy protocol when connecting to the Java
//     server", while remote.use-proxy-protocol plus a root use-proxy-protocol on that same
//     config are dropped silently (no warning, no rewrite to true).
//   - That Gate's Geyser listener honours the header; that policy is covered by
//     pkg/edition/bedrock/geyser/geyser_proxy_protocol_test.go.

const (
	geyserConfigTemplate = "pkg/configs/config.yml"
	bedrockGuideDoc      = ".web/docs/guide/bedrock.md"

	// geyser2HaproxyPath is the only path Geyser 2.x reads the Java-side HAProxy toggle
	// from: advanced.java.use-haproxy-protocol.
	geyser2HaproxyPath = "advanced.java.use-haproxy-protocol"
	// geyser1HaproxyKey is the pre-2.0 key. Geyser migrates it (ConfigMigrations
	// addVersion(5)), so it stays documented - but only as the legacy shape, never as
	// the key an operator of a current Geyser should add.
	geyser1HaproxyKey = "remote.use-proxy-protocol"
)

func TestShippedConfigTemplateNamesGeyser2HaproxyPath(t *testing.T) {
	template := readRepoFile(t, geyserConfigTemplate)

	// The template is generated from the repo-root config.yml (`make config`,
	// go:generate), so a guidance fix has to land in both or the shipped copy drifts.
	if root := readRepoFile(t, "config.yml"); root != template {
		t.Errorf("config.yml and %s differ; %s is generated from the root copy, so both "+
			"must carry the Geyser guidance", geyserConfigTemplate, geyserConfigTemplate)
	}

	if !strings.Contains(template, geyser2HaproxyPath) {
		t.Errorf("%s does not name %s in its manual Geyser setup comment; "+
			"external-Geyser operators are then left to guess a key Geyser 2.x drops",
			geyserConfigTemplate, geyser2HaproxyPath)
	}
	// The sentence this guard replaced told operators to set a bare
	// "use-proxy-protocol: true", which a sectioned Geyser 2.x config ignores.
	if strings.Contains(template, "and use-proxy-protocol: true in Geyser config") {
		t.Errorf("%s still tells operators to set a bare use-proxy-protocol: true",
			geyserConfigTemplate)
	}
	// Version-awareness: a pre-2.0 Geyser build has no advanced.java section.
	if !strings.Contains(template, geyser1HaproxyKey) {
		t.Errorf("%s does not record the pre-2.0 key %s, so the guidance no longer tells "+
			"readers why an older Geyser config differs", geyserConfigTemplate, geyser1HaproxyKey)
	}
}

func TestBedrockGuideTeachesGeyser2HaproxyPath(t *testing.T) {
	doc := readRepoFile(t, bedrockGuideDoc)

	// Two operator surfaces must carry it: the manual-setup snippet and the
	// "proxy protocol settings" troubleshooting step.
	atPath := 0
	for i, block := range fencedBlocks(doc) {
		for _, path := range yamlKeyPaths(block) {
			if path != geyser2HaproxyPath {
				continue
			}
			atPath++
			if value := yamlValue(block, "use-haproxy-protocol"); value != "true" {
				t.Errorf("%s yaml block %d sets %s to %q, want true:\n%s",
					bedrockGuideDoc, i+1, geyser2HaproxyPath, value, block)
			}
		}
	}
	if atPath < 2 {
		t.Errorf("%s sets %s in %d snippet(s), want the manual-setup snippet and the "+
			"troubleshooting step", bedrockGuideDoc, geyser2HaproxyPath, atPath)
	}

	// The dotted path must also be spelled out in prose, for a reader who greps the page.
	if !strings.Contains(doc, geyser2HaproxyPath) {
		t.Errorf("%s never spells out %s", bedrockGuideDoc, geyser2HaproxyPath)
	}
	if !strings.Contains(doc, geyser1HaproxyKey) {
		t.Errorf("%s does not mention the pre-2.0 key %s, so the guidance no longer "+
			"tells readers why their old config differs", bedrockGuideDoc, geyser1HaproxyKey)
	}
	if !strings.Contains(doc, "config-version") {
		t.Errorf("%s does not name Geyser's config-version, so the claim that %s is "+
			"the Geyser 2.x path is not tied to the version it is true for",
			bedrockGuideDoc, geyser2HaproxyPath)
	}

	// No copyable snippet may teach a legacy proxy-protocol key: those are dropped without
	// a warning on a config that Geyser has already migrated.
	for i, block := range fencedBlocks(doc) {
		for _, path := range yamlKeyPaths(block) {
			if path == "use-proxy-protocol" || strings.HasSuffix(path, ".use-proxy-protocol") {
				t.Errorf("%s yaml block %d still teaches the legacy key %q; the Geyser 2.x "+
					"path is %s:\n%s", bedrockGuideDoc, i+1, path, geyser2HaproxyPath, block)
			}
		}
	}
}

// fencedBlocks returns the contents of every ```yaml fence in the document.
func fencedBlocks(doc string) []string {
	var (
		blocks  []string
		current []string
		open    bool
	)
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !open && strings.HasPrefix(trimmed, "```yaml"):
			open, current = true, nil
		case open && trimmed == "```":
			blocks = append(blocks, strings.Join(current, "\n"))
			open = false
		case open:
			current = append(current, line)
		}
	}
	return blocks
}

// yamlKeyPaths returns the dotted path of every mapping key in a YAML block, resolved by
// indentation. Comment lines are skipped so a commented-out key is not mistaken for a
// live one.
func yamlKeyPaths(block string) []string {
	type frame struct {
		indent int
		key    string
	}
	var (
		stack []frame
		paths []string
	)
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		key, _, ok := strings.Cut(trimmed, ":")
		if !ok || strings.ContainsAny(key, " \t") {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, frame{indent: indent, key: key})
		parts := make([]string, 0, len(stack))
		for _, f := range stack {
			parts = append(parts, f.key)
		}
		paths = append(paths, strings.Join(parts, "."))
	}
	return paths
}

// yamlValue returns the value of the first live (non-comment) line in block whose key is
// name, with surrounding space trimmed.
func yamlValue(block, name string) string {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if key, value, ok := strings.Cut(trimmed, ":"); ok && strings.TrimSpace(key) == name {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
