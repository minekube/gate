package analyze

import (
	"fmt"
	"go/types"
	"slices"

	"golang.org/x/tools/go/packages"
)

func packageDeclarations(pkg *packages.Package) ([]Declaration, error) {
	entries, err := packageDeclarationEntries(pkg)
	if err != nil {
		return nil, err
	}
	declarations := make([]Declaration, len(entries))
	for index, entry := range entries {
		declarations[index] = entry.Declaration
	}
	return declarations, nil
}

type declarationEntry struct {
	Declaration Declaration
	Object      types.Object
	Selection   *types.Selection
	Type        types.Type
}

func packageDeclarationEntries(pkg *packages.Package) ([]declarationEntry, error) {
	if pkg.Types == nil {
		return nil, fmt.Errorf("package %q has no type information", pkg.PkgPath)
	}
	scope := pkg.Types.Scope()
	names := scope.Names()
	slices.Sort(names)

	var entries []declarationEntry
	for _, name := range names {
		object := scope.Lookup(name)
		if object == nil || !object.Exported() {
			continue
		}
		declaration, ok := objectDeclaration(pkg.PkgPath, object)
		if !ok {
			continue
		}
		entries = append(entries, declarationEntry{
			Declaration: declaration,
			Object:      object,
			Type:        object.Type(),
		})

		typeName, ok := object.(*types.TypeName)
		if !ok || typeName.IsAlias() {
			continue
		}
		named, ok := types.Unalias(typeName.Type()).(*types.Named)
		if !ok {
			continue
		}
		entries = append(
			entries,
			methodDeclarationEntries(pkg.PkgPath, typeName.Name(), named)...,
		)
	}
	slices.SortFunc(entries, func(left, right declarationEntry) int {
		return compareDeclarations(left.Declaration, right.Declaration)
	})
	return entries, nil
}

func objectDeclaration(packagePath string, object types.Object) (Declaration, bool) {
	declaration := Declaration{
		Identity:    packagePath + "." + object.Name(),
		PackagePath: packagePath,
		Name:        object.Name(),
	}
	switch object := object.(type) {
	case *types.Const:
		declaration.Kind = DeclarationConstant
	case *types.Func:
		if object.Signature().Recv() != nil {
			return Declaration{}, false
		}
		declaration.Kind = DeclarationFunction
	case *types.TypeName:
		if object.IsAlias() {
			declaration.Kind = DeclarationAlias
		} else {
			declaration.Kind = DeclarationType
		}
	case *types.Var:
		if object.IsField() {
			return Declaration{}, false
		}
		declaration.Kind = DeclarationVariable
	default:
		return Declaration{}, false
	}
	return declaration, true
}

func methodDeclarations(
	packagePath string,
	receiver string,
	named *types.Named,
) []Declaration {
	entries := methodDeclarationEntries(packagePath, receiver, named)
	declarations := make([]Declaration, len(entries))
	for index, entry := range entries {
		declarations[index] = entry.Declaration
	}
	return declarations
}

func methodDeclarationEntries(
	packagePath string,
	receiver string,
	named *types.Named,
) []declarationEntry {
	entries := make(map[string]declarationEntry)
	add := func(methodSet *types.MethodSet, pointerReceiver bool) {
		for index := range methodSet.Len() {
			selection := methodSet.At(index)
			method, ok := selection.Obj().(*types.Func)
			if !ok || !method.Exported() {
				continue
			}
			identity := packagePath + "." + receiver + "." + method.Name()
			if _, exists := entries[identity]; exists {
				continue
			}
			entries[identity] = declarationEntry{
				Declaration: Declaration{
					Identity:        identity,
					PackagePath:     packagePath,
					Name:            method.Name(),
					Receiver:        receiver,
					PointerReceiver: pointerReceiver,
					Kind:            DeclarationMethod,
				},
				Object:    method,
				Selection: selection,
				Type:      method.Type(),
			}
		}
	}
	add(types.NewMethodSet(named), false)
	add(types.NewMethodSet(types.NewPointer(named)), true)

	result := make([]declarationEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	slices.SortFunc(result, func(left, right declarationEntry) int {
		return compareDeclarations(left.Declaration, right.Declaration)
	})
	return result
}
