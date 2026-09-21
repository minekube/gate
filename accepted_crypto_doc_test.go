package main

import (
	"os"
	"strings"
	"testing"
)

// Guard over the scanner-triage record docs/accepted-crypto-primitives.md.
//
// WHAT THESE TESTS PROVE
//
//   - The record exists and names every accepted site by source path and symbol, so a
//     triager arriving from a scanner alert lands on a rationale instead of archaeology.
//   - AGENTS.md points at the record, so it is discoverable from the project memory the
//     same way the Velocity sync record is.
//   - Each accepted site still carries the in-code pointer comment, and the symbol the
//     record names still exists there. A record that outlives the code it describes is
//     worse than none: the next triager trusts a stale exemption. This catches a site
//     being renamed, deleted, or regenerated without the pointer moving with it.
//   - The three protocol-mandated primitives (Mojang hasJoined serverId, vanilla offline
//     UUIDv3, Floodgate's v5 mapping) are still the primitives the record accepts. Those
//     are protocol constants: a change there is an interoperability change, not a hygiene
//     cleanup, so it must update this record deliberately. The fourth site (JsonHash, an
//     in-process change fingerprint with no security consequence) is intentionally NOT
//     pinned to SHA-1 - the record explicitly allows swapping it for SHA-256, and this
//     test must not forbid what the record permits.
//
// WHAT THEY DO NOT PROVE
//
//   - That the primitives are correct for the protocol. That claim lives in the record and
//     in the review that produced it, not in a string match.

const acceptedCryptoDocPath = "docs/accepted-crypto-primitives.md"

// acceptedCryptoSites lists the four accepted sites: the source file, the symbol the
// pointer comment must sit on, and (where the record pins the primitive) a marker that
// must still be present in that file. An empty marker means the record permits changing
// the primitive, so nothing is asserted about it.
var acceptedCryptoSites = []struct {
	name   string
	file   string
	symbol string
	marker string
}{
	{
		name:   "Mojang hasJoined serverId",
		file:   "pkg/edition/java/auth/authenticator.go",
		symbol: "GenerateServerID",
		marker: "sha1.New()",
	},
	{
		name:   "vanilla offline UUIDv3",
		file:   "pkg/util/uuid/uuid.go",
		symbol: "OfflinePlayerUUID",
		marker: "md5.Sum(",
	},
	{
		name:   "Gate/Connect Bedrock XUID v5 mapping",
		file:   "pkg/edition/bedrock/geyser/floodgate/floodgate.go",
		symbol: "JavaUuid",
		marker: "sha1.New()",
	},
	{
		name:   "in-process config fingerprint",
		file:   "pkg/internal/hashutil/util.go",
		symbol: "JsonHash",
		marker: "", // SHA-256 swap is explicitly permitted by the record
	},
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// TestAcceptedCryptoRecordNamesEverySite is the record's own content contract: every
// accepted site is named by path and symbol, and the record states why the primitive is
// fixed rather than merely asserting that it is accepted.
func TestAcceptedCryptoRecordNamesEverySite(t *testing.T) {
	record := readRepoFile(t, acceptedCryptoDocPath)

	for _, site := range acceptedCryptoSites {
		if !strings.Contains(record, site.file) {
			t.Errorf("%s does not name the site path %s (%s); an unnamed exemption cannot be "+
				"triaged and will be re-reported", acceptedCryptoDocPath, site.file, site.name)
		}
		if !strings.Contains(record, site.symbol) {
			t.Errorf("%s does not name the symbol %s in %s (%s); a path-only entry rots the "+
				"moment the file moves", acceptedCryptoDocPath, site.symbol, site.file, site.name)
		}
	}

	// The point of the record is the justification, not the list. Require the protocol
	// anchors a future triager has to be able to cite.
	for _, anchor := range []string{
		"hasJoined",
		"UUIDv3",
		"UUIDv5",
		"RFC 4122",
		"suppress",
	} {
		if !strings.Contains(record, anchor) {
			t.Errorf("%s no longer states %q; the record must keep the protocol justification "+
				"and the suppression guidance, not just a list of files", acceptedCryptoDocPath, anchor)
		}
	}
}

// TestAgentsMemoryPointsAtAcceptedCryptoRecord mirrors the Velocity sync guard: a committed
// file nobody is pointed at is only marginally better than the archaeology it replaced.
func TestAgentsMemoryPointsAtAcceptedCryptoRecord(t *testing.T) {
	agents := readRepoFile(t, "AGENTS.md")
	if !strings.Contains(agents, acceptedCryptoDocPath) {
		t.Errorf("AGENTS.md does not mention %s; the record needs one pointer from the project "+
			"memory to be found without archaeology", acceptedCryptoDocPath)
	}
}

// TestAcceptedCryptoSitesCarryPointerComment keeps the record and the code in step: each
// accepted site must still point at the record from the source, at the symbol the record
// names, and the pinned primitives must still be the accepted ones.
func TestAcceptedCryptoSitesCarryPointerComment(t *testing.T) {
	for _, site := range acceptedCryptoSites {
		src := readRepoFile(t, site.file)

		if !strings.Contains(src, site.symbol) {
			t.Errorf("%s no longer defines %s but %s still exempts it (%s); remove the site from "+
				"the record or move the pointer with the code", site.file, site.symbol,
				acceptedCryptoDocPath, site.name)
			continue
		}
		if !strings.Contains(src, acceptedCryptoDocPath) {
			t.Errorf("%s does not point at %s (%s); a triager reading the flagged line must find "+
				"the rationale there, not only in AGENTS.md", site.file, acceptedCryptoDocPath, site.name)
		}
		if site.marker != "" && !strings.Contains(src, site.marker) {
			t.Errorf("%s no longer contains %q (%s). The record accepts this primitive because the "+
				"protocol fixes it, so a change here has to update %s in the same change",
				site.file, site.marker, site.name, acceptedCryptoDocPath)
		}
	}
}
