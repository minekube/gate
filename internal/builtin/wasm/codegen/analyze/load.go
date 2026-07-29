package analyze

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Options controls public Go API discovery.
type Options struct {
	Dir        string
	ModulePath string
	Patterns   []string
	Exclusions []Exclusion
}

// Result is the public declaration inventory produced by Load.
type Result struct {
	Packages     []string
	Declarations []Declaration
	Excluded     []ExcludedDeclaration
}

// DeclarationKind identifies a public Go declaration.
type DeclarationKind string

const (
	DeclarationAlias    DeclarationKind = "alias"
	DeclarationConstant DeclarationKind = "constant"
	DeclarationFunction DeclarationKind = "function"
	DeclarationMethod   DeclarationKind = "method"
	DeclarationType     DeclarationKind = "type"
	DeclarationVariable DeclarationKind = "variable"
)

// Declaration identifies an exported Go declaration.
type Declaration struct {
	Identity        string
	PackagePath     string
	Name            string
	Receiver        string
	PointerReceiver bool
	Kind            DeclarationKind
}

// Load discovers the complete public declaration inventory.
func Load(ctx context.Context, options Options) (*Result, error) {
	loaded, err := loadPublicPackages(ctx, options)
	if err != nil {
		return nil, err
	}

	result := &Result{}
	seenDeclarations := make(map[string]struct{})
	for _, pkg := range loaded {
		result.Packages = append(result.Packages, pkg.PkgPath)
		declarations, err := packageDeclarations(pkg)
		if err != nil {
			return nil, err
		}
		for _, declaration := range declarations {
			if _, exists := seenDeclarations[declaration.Identity]; exists {
				return nil, fmt.Errorf(
					"duplicate public declaration identity %q",
					declaration.Identity,
				)
			}
			seenDeclarations[declaration.Identity] = struct{}{}
			result.Declarations = append(result.Declarations, declaration)
		}
	}
	slices.Sort(result.Packages)
	slices.SortFunc(result.Declarations, compareDeclarations)

	if err := applyExclusions(result, options.Exclusions); err != nil {
		return nil, err
	}
	return result, nil
}

func loadPublicPackages(
	ctx context.Context,
	options Options,
) ([]*packages.Package, error) {
	if options.Dir == "" {
		return nil, fmt.Errorf("analysis directory is required")
	}
	if options.ModulePath == "" {
		return nil, fmt.Errorf("analysis module path is required")
	}
	if len(options.Patterns) == 0 {
		return nil, fmt.Errorf("at least one package pattern is required")
	}
	if err := validateExclusions(options.Exclusions); err != nil {
		return nil, err
	}

	cfg := &packages.Config{
		Context: ctx,
		Dir:     options.Dir,
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps,
		Tests: false,
	}
	loaded, err := packages.Load(cfg, options.Patterns...)
	if err != nil {
		return nil, fmt.Errorf("load public Gate packages: %w", err)
	}
	if err := packageErrors(loaded); err != nil {
		return nil, err
	}

	seenPackages := make(map[string]struct{})
	publicPackages := make([]*packages.Package, 0, len(loaded))
	for _, pkg := range loaded {
		if !inPublicScope(pkg.PkgPath, options.ModulePath) {
			continue
		}
		if _, exists := seenPackages[pkg.PkgPath]; exists {
			continue
		}
		seenPackages[pkg.PkgPath] = struct{}{}
		publicPackages = append(publicPackages, pkg)
	}
	slices.SortFunc(publicPackages, func(left, right *packages.Package) int {
		return strings.Compare(left.PkgPath, right.PkgPath)
	})
	return publicPackages, nil
}

// GateOptions returns the production Gate API discovery options.
func GateOptions(dir string) Options {
	const modulePath = "go.minekube.com/gate"
	return Options{
		Dir:        dir,
		ModulePath: modulePath,
		Patterns:   []string{"./api/...", "./pkg/..."},
		Exclusions: NativeBootstrapExclusions(modulePath),
	}
}

func packageErrors(roots []*packages.Package) error {
	var diagnostics []string
	packages.Visit(roots, nil, func(pkg *packages.Package) {
		for _, diagnostic := range pkg.Errors {
			diagnostics = append(diagnostics, diagnostic.Error())
		}
	})
	if len(diagnostics) == 0 {
		return nil
	}
	slices.Sort(diagnostics)
	return fmt.Errorf("load public Gate packages:\n%s", strings.Join(diagnostics, "\n"))
}

func inPublicScope(importPath, modulePath string) bool {
	if importPath == "" || importPath == "command-line-arguments" {
		return false
	}
	relative := strings.TrimPrefix(importPath, modulePath+"/")
	if relative == importPath {
		return false
	}
	segments := strings.Split(relative, "/")
	if len(segments) < 2 || (segments[0] != "api" && segments[0] != "pkg") {
		return false
	}
	return !slices.Contains(segments, "internal") && path.Base(importPath) != "main"
}

func compareDeclarations(left, right Declaration) int {
	return strings.Compare(left.Identity, right.Identity)
}
