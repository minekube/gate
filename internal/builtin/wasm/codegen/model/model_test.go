package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSortsSetsAndRejectsDuplicateIdentities(t *testing.T) {
	api := API{
		FormatVersion: 1,
		ModulePath:    "example.com/project",
		Packages: []Package{
			{
				Path:         "example.com/project/pkg/z",
				Name:         "z",
				WITName:      "pkg-z",
				Declarations: []string{"z.B", "z.A"},
			},
			{
				Path:         "example.com/project/pkg/a",
				Name:         "a",
				WITName:      "pkg-a",
				Declarations: []string{"a.A"},
			},
		},
		Declarations: []Declaration{
			{
				Identity:     "z.B",
				PackagePath:  "example.com/project/pkg/z",
				GoName:       "B",
				WITName:      "b",
				Kind:         DeclarationFunction,
				Coverage:     Coverage{State: CoverageRepresented},
				Dependencies: []string{"z.TypeB", "z.TypeA", "z.TypeB"},
				Callable: &Callable{
					Parameters: []Parameter{
						{GoName: "second", WITName: "second"},
						{GoName: "first", WITName: "first"},
					},
				},
			},
			{
				Identity:    "a.A",
				PackagePath: "example.com/project/pkg/a",
				GoName:      "A",
				WITName:     "a",
				Kind:        DeclarationType,
				Coverage:    Coverage{State: CoverageRepresented},
			},
			{
				Identity:    "z.A",
				PackagePath: "example.com/project/pkg/z",
				GoName:      "A",
				WITName:     "a",
				Kind:        DeclarationType,
				Coverage:    Coverage{State: CoverageRepresented},
			},
		},
	}

	require.NoError(t, api.Normalize())
	require.Equal(t, []string{
		"example.com/project/pkg/a",
		"example.com/project/pkg/z",
	}, []string{api.Packages[0].Path, api.Packages[1].Path})
	require.Equal(t, []string{"z.A", "z.B"}, api.Packages[1].Declarations)
	require.Equal(t, []string{"a.A", "z.A", "z.B"}, []string{
		api.Declarations[0].Identity,
		api.Declarations[1].Identity,
		api.Declarations[2].Identity,
	})
	require.Equal(t, []string{"z.TypeA", "z.TypeB"}, api.Declarations[2].Dependencies)
	require.Equal(t, "second", api.Declarations[2].Callable.Parameters[0].GoName,
		"semantically ordered parameter lists must not be reordered")

	encoded, err := json.Marshal(api)
	require.NoError(t, err)
	var decoded API
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, api, decoded)

	api.Declarations = append(api.Declarations, api.Declarations[0])
	require.ErrorContains(t, api.Normalize(), "duplicate declaration identity")
}

func TestNormalizeRejectsWITNameCollisionWithinPackage(t *testing.T) {
	api := API{
		FormatVersion: 1,
		ModulePath:    "example.com/project",
		Packages: []Package{{
			Path:    "example.com/project/pkg/a",
			Name:    "a",
			WITName: "pkg-a",
		}},
		Declarations: []Declaration{
			{
				Identity:    "a.URL",
				PackagePath: "example.com/project/pkg/a",
				GoName:      "URL",
				WITName:     "url",
				Kind:        DeclarationType,
				Coverage:    Coverage{State: CoverageRepresented},
			},
			{
				Identity:    "a.Url",
				PackagePath: "example.com/project/pkg/a",
				GoName:      "Url",
				WITName:     "url",
				Kind:        DeclarationType,
				Coverage:    Coverage{State: CoverageRepresented},
			},
		},
	}

	require.ErrorContains(t, api.Normalize(), "WIT name collision")
}
