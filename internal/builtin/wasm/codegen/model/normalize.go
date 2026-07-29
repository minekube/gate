package model

import (
	"fmt"
	"slices"
	"strings"
)

// Normalize validates and deterministically orders the canonical model.
func (api *API) Normalize() error {
	if api.FormatVersion == 0 {
		return fmt.Errorf("canonical model format version must be non-zero")
	}
	if api.ModulePath == "" {
		return fmt.Errorf("canonical model module path is required")
	}

	slices.SortFunc(api.Packages, func(left, right Package) int {
		return strings.Compare(left.Path, right.Path)
	})
	packagePaths := make(map[string]struct{}, len(api.Packages))
	packageWITNames := make(map[string]string, len(api.Packages))
	for index := range api.Packages {
		pkg := &api.Packages[index]
		if pkg.Path == "" || pkg.WITName == "" {
			return fmt.Errorf("canonical package path and WIT name are required")
		}
		if _, exists := packagePaths[pkg.Path]; exists {
			return fmt.Errorf("duplicate package path %q", pkg.Path)
		}
		packagePaths[pkg.Path] = struct{}{}
		if previous, exists := packageWITNames[pkg.WITName]; exists {
			return fmt.Errorf(
				"package WIT name collision %q between %q and %q",
				pkg.WITName,
				previous,
				pkg.Path,
			)
		}
		packageWITNames[pkg.WITName] = pkg.Path
		pkg.Declarations = sortedUnique(pkg.Declarations)
	}

	slices.SortFunc(api.Declarations, func(left, right Declaration) int {
		return strings.Compare(left.Identity, right.Identity)
	})
	identities := make(map[string]struct{}, len(api.Declarations))
	witNames := make(map[string]string, len(api.Declarations))
	for index := range api.Declarations {
		declaration := &api.Declarations[index]
		if declaration.Identity == "" {
			return fmt.Errorf("canonical declaration identity is required")
		}
		if _, exists := identities[declaration.Identity]; exists {
			return fmt.Errorf(
				"duplicate declaration identity %q",
				declaration.Identity,
			)
		}
		identities[declaration.Identity] = struct{}{}
		if _, exists := packagePaths[declaration.PackagePath]; !exists {
			return fmt.Errorf(
				"declaration %q refers to unknown package %q",
				declaration.Identity,
				declaration.PackagePath,
			)
		}
		if declaration.WITName == "" {
			return fmt.Errorf(
				"declaration %q has no WIT name",
				declaration.Identity,
			)
		}
		witIdentity := declaration.PackagePath + "." + declaration.WITName
		if previous, exists := witNames[witIdentity]; exists {
			return fmt.Errorf(
				"WIT name collision %q between %q and %q",
				declaration.WITName,
				previous,
				declaration.Identity,
			)
		}
		witNames[witIdentity] = declaration.Identity
		switch declaration.Coverage.State {
		case CoverageRepresented, CoverageExcluded:
		default:
			return fmt.Errorf(
				"declaration %q has invalid coverage state %q",
				declaration.Identity,
				declaration.Coverage.State,
			)
		}
		if declaration.Coverage.State == CoverageExcluded &&
			strings.TrimSpace(declaration.Coverage.Reason) == "" {
			return fmt.Errorf(
				"excluded declaration %q must include a reason",
				declaration.Identity,
			)
		}
		declaration.Dependencies = sortedUnique(declaration.Dependencies)
	}
	return nil
}

func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := slices.Clone(values)
	slices.Sort(result)
	return slices.Compact(result)
}
