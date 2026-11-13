package main

import (
	"regexp"
	"strings"
	"testing"
)

// Guards over the operator-facing claims that the shipped config keys are
// documented where an operator would look for them.
//
// WHAT THESE TESTS PROVE
//
//   - Every config key added since v0.76 is reachable from the guide, not only
//     from the commented template: `status.versionName` (v0.77.0) is taught in
//     .web/docs/guide/config/index.md and `config.builtinCommandRefusedMessage`
//     (v0.78.0) is taught in .web/docs/guide/builtin-commands.md, each with the
//     behaviour an operator has to know (default unchanged, display-only).
//   - The shipped templates still carry both keys, so a template edit that drops
//     one fails here instead of leaving the guide pointing at a key that no
//     longer exists. The templates are byte-identical copies (make sync-configs).
//   - The two modded-backend behaviour changes that a support case would
//     otherwise re-open as "Gate breaks modded servers" stay documented: the
//     backend's unmodelled status members survive a re-encode (v0.77.1) and an
//     undecodable backend command tree no longer closes the session (v0.76.3),
//     including the two consequences (no proxy-command injection, no
//     PlayerAvailableCommandsEvent) that must not be re-discovered as bugs.
//
// WHAT THEY DO NOT PROVE
//
//   - That Gate reads the keys. `status.versionName` is covered by
//     `TestNewInitialPingAdvertisedVersionName` in
//     pkg/edition/java/proxy, the refusal message by the config tests in
//     pkg/edition/java/config, and the status/command-tree behaviour by
//     pkg/edition/java/ping and pkg/edition/java/proxy respectively. These are
//     documentation contracts, not behaviour tests.
//   - That the pages render. `.web`'s VitePress build is the check for that.

const (
	guideConfigDoc          = ".web/docs/guide/config/index.md"
	guideBuiltinCommandsDoc = ".web/docs/guide/builtin-commands.md"
	guideModdedServersDoc   = ".web/docs/guide/modded-servers.md"

	// The two byte-identical config templates: the repo root copy and the
	// embedded copy Gate writes with `gate config --write`.
	rootConfigTemplate     = "config.yml"
	embeddedConfigTemplate = "pkg/configs/config.yml"

	// The default refusal message. It must stay quoted verbatim in the guide:
	// operators search for the text they saw in-game.
	defaultBuiltinCommandRefusedMessage = `§cYou don't have permission to use this command (requires "{node}").`
)

func requireDocContains(t *testing.T, path string, want []string) {
	t.Helper()
	content := readRepoFile(t, path)
	for _, needle := range want {
		if !strings.Contains(content, needle) {
			t.Fatalf("%s no longer documents %q", path, needle)
		}
	}
}

func TestStatusVersionNameDocumented(t *testing.T) {
	requireDocContains(t, guideConfigDoc, []string{
		"## Server List Appearance",
		"versionName:",
		"**`versionName`** (v0.77.0 and newer)",
		// The two facts an operator needs: the default stays the version range
		// (so leaving it empty is not a regression), and the advertised
		// protocol/join set is untouched (so a custom name is not a filter).
		"Empty (the default) advertises Gate's supported version range",
		"The advertised `protocol` and the set of client versions",
	})

	// A template without the key would make the guide's snippet unfindable in
	// the file operators actually generate.
	requireDocContains(t, rootConfigTemplate, []string{"versionName:"})
	requireDocContains(t, embeddedConfigTemplate, []string{"versionName:"})
}

func TestBuiltinCommandRefusedMessageDocumented(t *testing.T) {
	requireDocContains(t, guideBuiltinCommandsDoc, []string{
		"### Customize the refusal message",
		"builtinCommandRefusedMessage:",
		"configurable since v0.78.0",
		// The token is the whole reason the message is configurable rather
		// than a fixed string; an example without it teaches nothing.
		"{node}` is replaced with the permission node",
		"The default is unchanged and needs no config",
		defaultBuiltinCommandRefusedMessage,
	})

	requireDocContains(t, rootConfigTemplate, []string{"builtinCommandRefusedMessage:"})
	requireDocContains(t, embeddedConfigTemplate, []string{"builtinCommandRefusedMessage:"})
}

func TestModdedBackendStatusAndCommandTreeDocumented(t *testing.T) {
	requireDocContains(t, guideModdedServersDoc, []string{
		// The status-member fix and the members it covers.
		"The backend's own mod data survives Gate's server-list answer",
		"`forgeData`",
		"`neoforgeData`",
		"Since v0.77.1",
		// The command-tree fix, the symptom it removes, and the two
		// consequences a reader would otherwise report as new bugs.
		"A command tree Gate cannot decode no longer ends the session",
		"Before v0.76.3",
		"`Internal server connection error`",
		"proxy commands are not injected",
		"`PlayerAvailableCommandsEvent` is not fired for it",
	})
}

// TestGeyserLiteBedrockCurrencyDocumented keeps the GeyserLite page honest about
// the Bedrock client line Gate's managed runtime actually carries.
//
// The claim is currency, not a fixed string: the version quoted on the page has
// to be the version Gate's go.mod pins. A GeyserLite bump (the Geyser/Bedrock
// currency leg of the release cascade) therefore fails here until the page
// names the new line, which is the drift that left the page at "no version at
// all" while Gate moved from GeyserLite v0.5.x to v0.7.6.
func TestGeyserLiteBedrockCurrencyDocumented(t *testing.T) {
	goMod := readRepoFile(t, "go.mod")
	m := regexp.MustCompile(`(?m)^\s*go\.minekube\.com/geyserlite v(\d+\.\d+\.\d+)`).FindStringSubmatch(goMod)
	if m == nil {
		t.Fatal("go.mod no longer pins go.minekube.com/geyserlite; update this guard with the new dependency path")
	}
	pinned := m[1]

	requireDocContains(t, ".web/docs/geyserlite/index.md", []string{
		"## Bedrock Transport and Client Currency",
		"Gate v0.76.2 pins GeyserLite v" + pinned,
		"native NetherNet ingress",
		"26.60",
		// The transport-notification defect is the one support case this page
		// has to pre-empt: a Bedrock listener that dies at open #257.
		"257th",
		"verified ingress queue overflow",
		"v0.75.10",
	})
}
