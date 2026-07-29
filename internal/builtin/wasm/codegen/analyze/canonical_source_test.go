package analyze

import (
	"go/token"
	"go/types"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func TestObjectSourceUsesStablePathsOutsideRepository(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/workspace/gate")
	tests := []struct {
		name        string
		filename    string
		packagePath string
		want        string
	}{
		{
			name:        "repository source",
			filename:    "/workspace/gate/pkg/example/example.go",
			packagePath: "go.minekube.com/gate/pkg/example",
			want:        "pkg/example/example.go",
		},
		{
			name:        "standard library source",
			filename:    "/opt/go/1.26.2/src/context/context.go",
			packagePath: "context",
			want:        "go/context",
		},
		{
			name:        "module cache source",
			filename:    "/home/user/go/pkg/mod/example.com/dependency@v1.2.3/pkg/api.go",
			packagePath: "example.com/dependency/pkg",
			want:        "go/example.com/dependency/pkg",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fileSet := token.NewFileSet()
			file := fileSet.AddFile(filepath.FromSlash(test.filename), -1, 100)
			file.AddLine(10)
			object := types.NewVar(
				file.Pos(10),
				types.NewPackage(test.packagePath, "fixture"),
				"Value",
				types.Typ[types.String],
			)

			source := objectSource(root, &packages.Package{Fset: fileSet}, object)

			require.Equal(t, test.want, source.File)
			if test.name == "repository source" {
				require.Equal(t, 2, source.Line)
				require.Equal(t, 1, source.Column)
			} else {
				require.Zero(t, source.Line)
				require.Zero(t, source.Column)
			}
		})
	}
}
