package analyze

import (
	"fmt"
	"go/types"
	"slices"

	"golang.org/x/tools/go/packages"
)

func packageDeclarations(pkg *packages.Package) ([]Declaration, error) {
	if pkg.Types == nil {
		return nil, fmt.Errorf("package %q has no type information", pkg.PkgPath)
	}
	scope := pkg.Types.Scope()
	names := scope.Names()
	slices.Sort(names)

	var declarations []Declaration
	for _, name := range names {
		object := scope.Lookup(name)
		if object == nil || !object.Exported() {
			continue
		}
		declaration, ok := objectDeclaration(pkg.PkgPath, object)
		if !ok {
			continue
		}
		declarations = append(declarations, declaration)

		typeName, ok := object.(*types.TypeName)
		if !ok || typeName.IsAlias() {
			continue
		}
		named, ok := types.Unalias(typeName.Type()).(*types.Named)
		if !ok {
			continue
		}
		declarations = append(
			declarations,
			methodDeclarations(pkg.PkgPath, typeName.Name(), named)...,
		)
	}
	slices.SortFunc(declarations, compareDeclarations)
	return declarations, nil
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
	declarations := make(map[string]Declaration)
	add := func(methodSet *types.MethodSet, pointerReceiver bool) {
		for index := range methodSet.Len() {
			selection := methodSet.At(index)
			method, ok := selection.Obj().(*types.Func)
			if !ok || !method.Exported() {
				continue
			}
			identity := packagePath + "." + receiver + "." + method.Name()
			if _, exists := declarations[identity]; exists {
				continue
			}
			declarations[identity] = Declaration{
				Identity:        identity,
				PackagePath:     packagePath,
				Name:            method.Name(),
				Receiver:        receiver,
				PointerReceiver: pointerReceiver,
				Kind:            DeclarationMethod,
			}
		}
	}
	add(types.NewMethodSet(named), false)
	add(types.NewMethodSet(types.NewPointer(named)), true)

	result := make([]Declaration, 0, len(declarations))
	for _, declaration := range declarations {
		result = append(result, declaration)
	}
	slices.SortFunc(result, compareDeclarations)
	return result
}
