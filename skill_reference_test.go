package main

import (
	"os"
	"os/exec"
	"testing"
)

// Go skips .agents when expanding ./..., so keep this entry point in the root
// package while the record checks live beside the skill they document.
func TestVelocitySyncReferenceSuite(t *testing.T) {
	const testPath = ".agents/skills/velocity-sync/references/velocity_sync_test.go"
	const recordPath = ".agents/skills/velocity-sync/references/VELOCITY_SYNC.md"
	// Opening both files here makes the outer test cache notice edits under .agents.
	for _, path := range []string{testPath, recordPath} {
		if _, err := os.ReadFile(path); err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
	}

	cmd := exec.Command("go", "test", "-count=1", "./"+testPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Velocity reference tests failed: %v\n%s", err, output)
	}
}
