package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// configDurationFields are the fields whose type is configutil.Duration.
//
// configutil.Duration already holds a Go duration: "30s" and 30000 both decode
// to the documented duration (pkg/util/configutil/duration.go). Multiplying one
// by a time unit therefore scales it a second time - the shipped 30s read
// timeout became ~347 days, the 5s connection timeout ~57 days, i.e. effectively
// no timeout at all, which is what let a stalled join wait forever.
//
// Add new configutil.Duration fields here so the guard keeps covering them.
var configDurationFields = regexp.MustCompile(`(ConnectionTimeout|ReadTimeout|Interval|CachePingTTL)`)

// scalingOperand matches the multiplying side of such a bug: a configured
// duration (optionally wrapped in a conversion) times a time unit.
func scalingOperand(text string) bool {
	return configDurationFields.MatchString(text)
}

// TestConfigDurationsAreNotRescaled guards the whole repository against scaling
// a configured duration a second time. It is the companion of the
// effective-deadline tests in pkg/edition/java/proxy (which pin the deadlines a
// connection really gets): this one keeps the same mistake from being
// re-introduced at a new call site, where no existing test would see it.
func TestConfigDurationsAreNotRescaled(t *testing.T) {
	var offenders []string
	fset := token.NewFileSet()

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", ".web", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || bin.Op != token.MUL {
				return true
			}
			left, right := exprSource(src, fset, bin.X), exprSource(src, fset, bin.Y)
			switch {
			case left == "time.Millisecond" && scalingOperand(right):
				offenders = append(offenders, fmt.Sprintf("%s: %s * %s", fset.Position(bin.Pos()), left, right))
			case right == "time.Millisecond" && scalingOperand(left):
				offenders = append(offenders, fmt.Sprintf("%s: %s * %s", fset.Position(bin.Pos()), left, right))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf("configured durations are scaled by time.Millisecond again (configutil.Duration already carries the unit):\n%s",
			strings.Join(offenders, "\n"))
	}
}

// exprSource returns the source text of an expression.
func exprSource(src []byte, fset *token.FileSet, e ast.Expr) string {
	start := fset.Position(e.Pos()).Offset
	end := fset.Position(e.End()).Offset
	if start < 0 || end > len(src) || start > end {
		return ""
	}
	return string(src[start:end])
}
