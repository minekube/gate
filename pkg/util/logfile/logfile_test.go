package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFile is a helper because the writer keeps its own handle open.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func rotated(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "gate.*.log"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return matches
}

func TestWriterAppendsAcrossReopen(t *testing.T) {
	dir := t.TempDir()

	w, err := Open(dir, 0, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err = w.Write([]byte("first\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err = w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// A restart must not truncate what the previous process wrote: this is the
	// whole point of the feature.
	w, err = Open(dir, 0, 0)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer w.Close()
	if _, err = w.Write([]byte("second\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got := readFile(t, filepath.Join(dir, FileName)); got != "first\nsecond\n" {
		t.Fatalf("content after restart = %q, want %q", got, "first\nsecond\n")
	}
}

func TestWriterCreatesNestedDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "logs")
	w, err := Open(dir, 0, 0)
	if err != nil {
		t.Fatalf("Open nested: %v", err)
	}
	defer w.Close()
	if _, err = w.Write([]byte("x\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, FileName)); got != "x\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestWriterRotatesAtMaxSize(t *testing.T) {
	dir := t.TempDir()
	// 200 bytes of headroom: the second write must rotate, the first must not.
	w, err := Open(dir, 0, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	w.maxSize = 200
	defer w.Close()

	line := strings.Repeat("a", 90) + "\n" // 91 bytes
	for i := 0; i < 4; i++ {
		if _, err = w.Write([]byte(line)); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
		// The writer must stay usable across a rotation.
		if i == 2 {
			if len(rotated(t, dir)) == 0 {
				t.Fatalf("no rotated file after crossing the size limit")
			}
		}
	}

	files := rotated(t, dir)
	if len(files) == 0 {
		t.Fatalf("expected at least one rotated file, got none")
	}
	// The active file only ever holds what was written after the last rotation.
	active := readFile(t, filepath.Join(dir, FileName))
	if len(active) > 200 {
		t.Fatalf("active file is %d bytes, above the 200-byte limit", len(active))
	}
	if !strings.Contains(readFile(t, files[0]), line) {
		t.Fatalf("rotated file does not contain a full line")
	}
}

func TestWriterPrunesOldRotations(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(dir, 0, 1) // keep one rotated file
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	w.maxSize = 120
	defer w.Close()
	// Each write is larger than the limit on its own, so every write rotates.
	for i := 0; i < 4; i++ {
		if _, err = w.Write([]byte(fmt.Sprintf("%091d\n", i))); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	if files := rotated(t, dir); len(files) > 1 {
		t.Fatalf("kept %d rotated files, want at most 1: %v", len(files), files)
	}
}
