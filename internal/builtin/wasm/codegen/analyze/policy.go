package analyze

import (
	"fmt"
	"slices"
	"strings"
)

// Exclusion is an explicit public API coverage decision.
type Exclusion struct {
	Identity string
	Reason   string
}

// ExcludedDeclaration records an exclusion that matched a real declaration.
type ExcludedDeclaration struct {
	Declaration Declaration
	Reason      string
}

// NativeBootstrapExclusions are the only initial declaration-specific
// exclusions. WebAssembly plugins use the generated component lifecycle.
func NativeBootstrapExclusions(modulePath string) []Exclusion {
	const reason = "native plugin bootstrap is replaced by the WebAssembly component lifecycle"
	return []Exclusion{
		{
			Identity: modulePath + "/pkg/edition/java/proxy.Plugin",
			Reason:   reason,
		},
		{
			Identity: modulePath + "/pkg/edition/java/proxy.Plugins",
			Reason:   reason,
		},
	}
}

func validateExclusions(exclusions []Exclusion) error {
	seen := make(map[string]struct{}, len(exclusions))
	for _, exclusion := range exclusions {
		if exclusion.Identity == "" {
			return fmt.Errorf("public API exclusion identity is required")
		}
		if strings.TrimSpace(exclusion.Reason) == "" {
			return fmt.Errorf(
				"public API exclusion %q must include a reason",
				exclusion.Identity,
			)
		}
		if _, exists := seen[exclusion.Identity]; exists {
			return fmt.Errorf(
				"duplicate public API exclusion %q",
				exclusion.Identity,
			)
		}
		seen[exclusion.Identity] = struct{}{}
	}
	return nil
}

func applyExclusions(result *Result, exclusions []Exclusion) error {
	byIdentity := make(map[string]Exclusion, len(exclusions))
	for _, exclusion := range exclusions {
		byIdentity[exclusion.Identity] = exclusion
	}

	represented := result.Declarations[:0]
	matched := make(map[string]struct{}, len(exclusions))
	for _, declaration := range result.Declarations {
		exclusion, excluded := byIdentity[declaration.Identity]
		if !excluded {
			represented = append(represented, declaration)
			continue
		}
		matched[declaration.Identity] = struct{}{}
		result.Excluded = append(result.Excluded, ExcludedDeclaration{
			Declaration: declaration,
			Reason:      exclusion.Reason,
		})
	}
	result.Declarations = represented

	for _, exclusion := range exclusions {
		if _, ok := matched[exclusion.Identity]; !ok {
			return fmt.Errorf(
				"public API exclusion %q did not match a public declaration",
				exclusion.Identity,
			)
		}
	}
	slices.SortFunc(result.Excluded, func(left, right ExcludedDeclaration) int {
		return strings.Compare(left.Declaration.Identity, right.Declaration.Identity)
	})
	return nil
}
