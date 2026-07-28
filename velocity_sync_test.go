package main

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Guard over VELOCITY_SYNC.md, the committed record of Gate's verified Velocity
// sync point.
//
// WHAT THESE TESTS PROVE
//
//   - VELOCITY_SYNC.md exists at the repo root and its machine-readable block
//     parses as YAML, so the record cannot silently rot into an unparseable state.
//   - The required fields are present and well-formed: 40-hex SHAs, ISO dates, a
//     positive PR number, a non-empty upstream repo/branch/subject.
//   - The recorded Gate commits actually resolve to commits in THIS repository,
//     and the recorded merge commit really references the recorded PR number.
//     This is the one substantive claim check available offline, and it fails for
//     a real reason: a typo'd or fabricated Gate SHA, or a PR number that does not
//     match the merge that landed it.
//   - The file does not assert general parity with upstream. The record is only
//     ever a *verified sync point as of* one commit; a sentence claiming Gate is
//     broadly current with Velocity would be true when written and quietly false
//     later, which is the specific failure this record exists to avoid. Checked as
//     an exact-match blocklist, which is why VELOCITY_SYNC.md does not spell those
//     phrasings out itself.
//   - AGENTS.md points at the file, so it stays discoverable.
//
// WHAT THESE TESTS DO NOT PROVE — stated plainly, because a guard that implies
// more than it checks is worse than no guard:
//
//   - NOT that the recorded upstream SHA exists, or says what the file says it
//     says. It lives in PaperMC/Velocity, not here; resolving it needs network
//     access, and a unit test that reaches the network is flaky and ends up
//     skipped or deleted. That check is deliberately out of scope. A scheduled CI
//     job against the GitHub compare API is the right home for it, if wanted.
//   - NOT that the Gate commit actually implements the upstream commit's
//     behavior. That was established by hand (see the file) and no test can
//     re-derive it.
//   - NOT that the record is current. Nothing here notices upstream moving ahead.
//     The file is only as true as the last person who updated it; these tests
//     enforce shape and internal consistency, not truth.
//   - NOT, in CI, that the Gate SHAs resolve: the test job checks out shallow
//     (fetch-depth 1), so the git-backed assertions skip there and run locally and
//     in full clones. Skipping is stated rather than silently passing.
//
// This is the weak-but-honest guard, shipped deliberately in preference to a
// stronger-looking one that could not fail for the reason it exists.

const velocitySyncPath = "VELOCITY_SYNC.md"

type velocitySyncRecord struct {
	VerifiedSyncPoint velocitySyncPoint `yaml:"verified_sync_point"`
	Log               []velocitySyncLog `yaml:"log"`
}

type velocitySyncPoint struct {
	UpstreamRepo    string `yaml:"upstream_repo"`
	UpstreamBranch  string `yaml:"upstream_branch"`
	UpstreamCommit  string `yaml:"upstream_commit"`
	UpstreamSubject string `yaml:"upstream_subject"`
	UpstreamDate    string `yaml:"upstream_date"`
	GateCommit      string `yaml:"gate_commit"`
	GateMergeCommit string `yaml:"gate_merge_commit"`
	GatePR          int    `yaml:"gate_pr"`
	GateDate        string `yaml:"gate_date"`
	VerifiedOn      string `yaml:"verified_on"`
}

type velocitySyncLog struct {
	Date                string `yaml:"date"`
	Kind                string `yaml:"kind"`
	Summary             string `yaml:"summary"`
	UpstreamCommit      string `yaml:"upstream_commit"`
	GateCommit          string `yaml:"gate_commit"`
	GatePR              int    `yaml:"gate_pr"`
	UpstreamRange       string `yaml:"upstream_range"`
	UpstreamCommitCount int    `yaml:"upstream_commit_count"`
	Ported              string `yaml:"ported"`
}

var (
	fullSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	isoDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	yamlBlock      = regexp.MustCompile("(?s)```yaml\n(.*?)\n```")
)

func readVelocitySyncFile(t *testing.T) string {
	t.Helper()

	body, err := os.ReadFile(velocitySyncPath)
	if err != nil {
		t.Fatalf("%s must exist at the repo root: %v", velocitySyncPath, err)
	}
	return string(body)
}

func readVelocitySyncRecord(t *testing.T) velocitySyncRecord {
	t.Helper()

	match := yamlBlock.FindStringSubmatch(readVelocitySyncFile(t))
	if match == nil {
		t.Fatalf("%s has no ```yaml block; the record must stay machine-readable", velocitySyncPath)
	}

	var record velocitySyncRecord
	if err := yaml.Unmarshal([]byte(match[1]), &record); err != nil {
		t.Fatalf("%s yaml block does not parse: %v", velocitySyncPath, err)
	}
	return record
}

// TestVelocitySyncRecordIsWellFormed pins the shape of the pinned sync point.
// An unparseable or half-filled record is indistinguishable from no record.
func TestVelocitySyncRecordIsWellFormed(t *testing.T) {
	point := readVelocitySyncRecord(t).VerifiedSyncPoint

	for field, value := range map[string]string{
		"upstream_repo":    point.UpstreamRepo,
		"upstream_branch":  point.UpstreamBranch,
		"upstream_subject": point.UpstreamSubject,
	} {
		if strings.TrimSpace(value) == "" {
			t.Errorf("verified_sync_point.%s is empty", field)
		}
	}

	for field, sha := range map[string]string{
		"upstream_commit":   point.UpstreamCommit,
		"gate_commit":       point.GateCommit,
		"gate_merge_commit": point.GateMergeCommit,
	} {
		if !fullSHAPattern.MatchString(sha) {
			t.Errorf("verified_sync_point.%s = %q is not a full 40-hex SHA; abbreviated "+
				"SHAs go ambiguous as history grows", field, sha)
		}
	}

	for field, date := range map[string]string{
		"upstream_date": point.UpstreamDate,
		"gate_date":     point.GateDate,
		"verified_on":   point.VerifiedOn,
	} {
		if !isoDatePattern.MatchString(date) {
			t.Errorf("verified_sync_point.%s = %q is not an ISO YYYY-MM-DD date", field, date)
		}
	}

	if point.GatePR <= 0 {
		t.Errorf("verified_sync_point.gate_pr = %d; the record must cite the PR that landed it", point.GatePR)
	}
}

// TestVelocitySyncLogIsWellFormed guards the part of the record that has to keep
// growing. A single SHA answers "where are we"; the log answers "was upstream
// commit X ever considered", which is the coverage question the pinned SHA
// cannot. Entries that lose their shape stop answering it.
func TestVelocitySyncLogIsWellFormed(t *testing.T) {
	record := readVelocitySyncRecord(t)

	if len(record.Log) == 0 {
		t.Fatal("log is empty; it must at minimum record the entry that established the pinned sync point")
	}

	for i, entry := range record.Log {
		if !isoDatePattern.MatchString(entry.Date) {
			t.Errorf("log[%d].date = %q is not an ISO YYYY-MM-DD date", i, entry.Date)
		}
		if strings.TrimSpace(entry.Summary) == "" {
			t.Errorf("log[%d].summary is empty; an entry with no reason answers nothing", i)
		}

		switch entry.Kind {
		case "sync":
			if !fullSHAPattern.MatchString(entry.UpstreamCommit) {
				t.Errorf("log[%d].upstream_commit = %q is not a full 40-hex SHA", i, entry.UpstreamCommit)
			}
			if !fullSHAPattern.MatchString(entry.GateCommit) {
				t.Errorf("log[%d].gate_commit = %q is not a full 40-hex SHA", i, entry.GateCommit)
			}
			if entry.GatePR <= 0 {
				t.Errorf("log[%d].gate_pr = %d; a sync entry must cite its PR", i, entry.GatePR)
			}
		case "review":
			if strings.TrimSpace(entry.UpstreamRange) == "" {
				t.Errorf("log[%d].upstream_range is empty; a review entry must say what it reviewed", i)
			}
			if entry.UpstreamCommitCount <= 0 {
				t.Errorf("log[%d].upstream_commit_count = %d; a review entry must count the commits it reviewed", i, entry.UpstreamCommitCount)
			}
			if strings.TrimSpace(entry.Ported) == "" {
				t.Errorf("log[%d].ported is empty; a review entry must say what it took, or 'none'", i)
			}
		default:
			t.Errorf("log[%d].kind = %q; want \"sync\" or \"review\"", i, entry.Kind)
		}
	}

	// The pinned point must itself appear in the log, or the log is not the
	// complete history it claims to be.
	pinned := record.VerifiedSyncPoint.UpstreamCommit
	var found bool
	for _, entry := range record.Log {
		if entry.Kind == "sync" && entry.UpstreamCommit == pinned {
			found = true
		}
	}
	if !found {
		t.Errorf("no sync log entry records the pinned upstream commit %s", pinned)
	}
}

// gitRevType resolves a revision in this repository, reporting whether the
// object is present at all. In a shallow clone (GitHub Actions checks out at
// fetch-depth 1) old objects are simply absent, which is not a record defect.
func gitRevType(t *testing.T, rev string) (string, bool) {
	t.Helper()

	out, err := exec.Command("git", "cat-file", "-t", rev).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func skipUnlessFullClone(t *testing.T) {
	t.Helper()

	out, err := exec.Command("git", "rev-parse", "--is-shallow-repository").Output()
	if err != nil {
		t.Skip("not a git repository (or git unavailable); the git-backed assertions cannot run here")
	}
	if strings.TrimSpace(string(out)) == "true" {
		t.Skip("shallow clone: historical objects are absent, so SHA resolution is not checkable here. " +
			"This assertion runs locally and in full clones.")
	}
}

// TestVelocitySyncRecordedGateCommitsResolve is the one substantive claim check
// that works offline: the Gate commits named in the record must exist in this
// repository, and the recorded merge commit must reference the recorded PR
// number. A transposed digit or a fabricated SHA fails here.
//
// Deliberately absent: any check of the upstream SHA, which lives in
// PaperMC/Velocity and would need the network. See the header comment.
func TestVelocitySyncRecordedGateCommitsResolve(t *testing.T) {
	skipUnlessFullClone(t)

	record := readVelocitySyncRecord(t)
	point := record.VerifiedSyncPoint

	revs := map[string]string{
		"verified_sync_point.gate_commit":       point.GateCommit,
		"verified_sync_point.gate_merge_commit": point.GateMergeCommit,
	}
	for _, entry := range record.Log {
		if entry.Kind == "sync" && entry.GateCommit != "" {
			revs["log gate_commit "+entry.GateCommit] = entry.GateCommit
		}
	}

	for field, rev := range revs {
		objType, ok := gitRevType(t, rev)
		if !ok {
			t.Errorf("%s = %s does not resolve in this repository", field, rev)
			continue
		}
		if objType != "commit" {
			t.Errorf("%s = %s resolves to a %s, not a commit", field, rev, objType)
		}
	}

	// The merge commit's subject carries "#<pr>" for a GitHub merge, which ties
	// the recorded PR number to the commit that actually landed the sync.
	subject, err := exec.Command("git", "log", "-1", "--format=%s", point.GateMergeCommit).Output()
	if err != nil {
		t.Fatalf("reading %s: %v", point.GateMergeCommit, err)
	}
	want := "#" + strconv.Itoa(point.GatePR)
	prToken := regexp.MustCompile(regexp.QuoteMeta(want) + `(?:[^0-9]|$)`)
	if !prToken.Match(subject) {
		t.Errorf("merge commit %s subject %q does not reference the recorded PR %s",
			point.GateMergeCommit, strings.TrimSpace(string(subject)), want)
	}
}

// TestVelocitySyncRecordClaimsOnlyAVerifiedPoint is the honesty guard, and the
// reason the file exists in this shape. We can prove one upstream commit landed;
// we cannot prove every earlier one did, and no record exists that could. The
// file must therefore only ever claim a *verified sync point as of* a commit.
// Any sentence upgrading that to general parity with upstream is the exact
// failure mode this record was written to avoid.
func TestVelocitySyncRecordClaimsOnlyAVerifiedPoint(t *testing.T) {
	body := strings.ToLower(readVelocitySyncFile(t))

	if !strings.Contains(body, "verified sync point as of") {
		t.Errorf("%s must state a %q; that precise phrasing is the claim we can defend",
			velocitySyncPath, "verified sync point as of")
	}

	// Exact-match blocklist. VELOCITY_SYNC.md deliberately does not spell these
	// out, so the guard can match literally without tripping over the prose
	// explaining it.
	for _, banned := range []string{
		"in sync with upstream",
		"in sync with velocity",
		"in sync with papermc",
		"gate is in sync",
		"up to date with upstream",
		"up to date with velocity",
		"fully ported",
		"fully synced",
		"fully in sync",
		"completely ported",
		"no drift",
		"gate is current with upstream",
		"ported every upstream",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("%s contains %q. The record can only claim a verified sync point as of one "+
				"commit; coverage of everything before it is unproven and unprovable from this repo",
				velocitySyncPath, banned)
		}
	}
}

// TestAgentsMemoryPointsAtVelocitySyncRecord keeps the record discoverable. A
// committed file nobody is pointed at is only marginally better than the
// archaeology it replaced.
func TestAgentsMemoryPointsAtVelocitySyncRecord(t *testing.T) {
	body, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("reading AGENTS.md: %v", err)
	}
	if !strings.Contains(string(body), velocitySyncPath) {
		t.Errorf("AGENTS.md does not mention %s; the record needs one pointer from the "+
			"project memory to be found without archaeology", velocitySyncPath)
	}
}
