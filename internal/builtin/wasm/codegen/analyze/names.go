package analyze

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// WITIdentifier converts a Go or package-path identifier to canonical WIT
// kebab-case.
func WITIdentifier(input string) string {
	runes := []rune(input)
	var words []string
	var word strings.Builder
	flush := func() {
		if word.Len() == 0 {
			return
		}
		words = append(words, word.String())
		word.Reset()
	}
	for index, current := range runes {
		if current > unicode.MaxASCII {
			flush()
			words = append(words, fmt.Sprintf("u%04x", current))
			continue
		}
		if !isASCIIAlphaNumeric(current) {
			flush()
			continue
		}

		var previous rune
		if index > 0 {
			previous = runes[index-1]
		}
		var next rune
		if index+1 < len(runes) {
			next = runes[index+1]
		}
		if unicode.IsUpper(current) && word.Len() > 0 &&
			(unicode.IsLower(previous) ||
				unicode.IsDigit(previous) ||
				(unicode.IsUpper(previous) && unicode.IsLower(next))) {
			flush()
		}
		word.WriteRune(unicode.ToLower(current))
	}
	flush()

	name := strings.Join(words, "-")
	if name == "" {
		name = "unnamed"
	}
	if first := rune(name[0]); unicode.IsDigit(first) {
		name = "n-" + name
	}
	if _, keyword := witKeywords[name]; keyword {
		name = "gate-" + name
	}
	return name
}

// PackageWITNames assigns deterministic collision-free WIT interface names.
func PackageWITNames(modulePath string, packagePaths []string) (map[string]string, error) {
	inputs := make([]NamedIdentity, 0, len(packagePaths))
	prefix := strings.TrimSuffix(modulePath, "/") + "/"
	for _, packagePath := range packagePaths {
		if !strings.HasPrefix(packagePath, prefix) {
			return nil, fmt.Errorf(
				"package path %q is outside module %q",
				packagePath,
				modulePath,
			)
		}
		relative := strings.TrimPrefix(packagePath, prefix)
		segments := strings.Split(relative, "/")
		for index := range segments {
			segments[index] = WITIdentifier(segments[index])
		}
		inputs = append(inputs, NamedIdentity{
			Identity: packagePath,
			GoName:   strings.Join(segments, "-"),
		})
	}
	return IdentityWITNames(inputs)
}

// NamedIdentity is a Go identity that needs a WIT name in one namespace.
type NamedIdentity struct {
	Identity string
	GoName   string
}

// IdentityWITNames assigns deterministic collision-free declaration names.
func IdentityWITNames(inputs []NamedIdentity) (map[string]string, error) {
	groups := make(map[string][]NamedIdentity)
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.Identity == "" {
			return nil, fmt.Errorf("Go identity is required for WIT naming")
		}
		if _, exists := seen[input.Identity]; exists {
			return nil, fmt.Errorf("duplicate Go identity %q", input.Identity)
		}
		seen[input.Identity] = struct{}{}
		base := WITIdentifier(input.GoName)
		groups[base] = append(groups[base], input)
	}

	result := make(map[string]string, len(inputs))
	assigned := make(map[string]string, len(inputs))
	for base, group := range groups {
		slices.SortFunc(group, func(left, right NamedIdentity) int {
			return strings.Compare(left.Identity, right.Identity)
		})
		for _, input := range group {
			name := base
			if len(group) > 1 {
				name += "-" + identityHash(input.Identity)
			}
			if previous, exists := assigned[name]; exists {
				return nil, fmt.Errorf(
					"WIT name hash collision %q between %q and %q",
					name,
					previous,
					input.Identity,
				)
			}
			assigned[name] = input.Identity
			result[input.Identity] = name
		}
	}
	return result, nil
}

func identityHash(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("%x", sum[:6])
}

func isASCIIAlphaNumeric(value rune) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
}

var witKeywords = map[string]struct{}{
	"as":          {},
	"bool":        {},
	"borrow":      {},
	"char":        {},
	"constructor": {},
	"enum":        {},
	"export":      {},
	"flags":       {},
	"float32":     {},
	"float64":     {},
	"from":        {},
	"func":        {},
	"future":      {},
	"import":      {},
	"include":     {},
	"interface":   {},
	"list":        {},
	"option":      {},
	"own":         {},
	"package":     {},
	"record":      {},
	"resource":    {},
	"result":      {},
	"s16":         {},
	"s32":         {},
	"s64":         {},
	"s8":          {},
	"static":      {},
	"stream":      {},
	"string":      {},
	"tuple":       {},
	"type":        {},
	"u16":         {},
	"u32":         {},
	"u64":         {},
	"u8":          {},
	"use":         {},
	"variant":     {},
	"with":        {},
	"world":       {},
}
