package analyze

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"go.minekube.com/gate/internal/wasm/model"
)

// Analyze loads and lowers Gate's complete public API into the canonical model.
func Analyze(ctx context.Context, options Options) (*model.API, error) {
	loaded, err := loadPublicPackages(ctx, options)
	if err != nil {
		return nil, err
	}
	packagePaths := make([]string, len(loaded))
	for index, pkg := range loaded {
		packagePaths[index] = pkg.PkgPath
	}
	packageNames, err := PackageWITNames(options.ModulePath, packagePaths)
	if err != nil {
		return nil, err
	}

	type packageAnalysis struct {
		pkg        *packages.Package
		entries    []declarationEntry
		documents  map[types.Object]string
		packageDoc string
	}
	analyses := make([]packageAnalysis, 0, len(loaded))
	allEntries := make([]declarationEntry, 0)
	for _, pkg := range loaded {
		entries, err := packageDeclarationEntries(pkg)
		if err != nil {
			return nil, err
		}
		documents, packageDoc := documentationIndex(pkg)
		analyses = append(analyses, packageAnalysis{
			pkg:        pkg,
			entries:    entries,
			documents:  documents,
			packageDoc: packageDoc,
		})
		allEntries = append(allEntries, entries...)
	}

	instances, err := collectGenericInstances(loaded)
	if err != nil {
		return nil, err
	}
	instanceEntries := make(map[string][]declarationEntry)
	for _, instance := range instances {
		if instance.Object == nil ||
			instance.Object.Pkg() == nil ||
			!instance.Object.Exported() ||
			!inPublicScope(instance.Object.Pkg().Path(), options.ModulePath) {
			continue
		}
		base, ok := objectDeclaration(instance.Object.Pkg().Path(), instance.Object)
		if !ok {
			continue
		}
		base.Identity = instance.InstanceIdentity
		entry := declarationEntry{
			Declaration: base,
			Object:      instance.Object,
			Type:        instance.Type,
		}
		instanceEntries[base.PackagePath] = append(
			instanceEntries[base.PackagePath],
			entry,
		)
		allEntries = append(allEntries, entry)
	}

	namesByPackage := make(map[string]map[string]string, len(loaded))
	for _, pkg := range loaded {
		var named []NamedIdentity
		for _, entry := range allEntries {
			if entry.Declaration.PackagePath != pkg.PkgPath {
				continue
			}
			named = append(named, NamedIdentity{
				Identity: entry.Declaration.Identity,
				GoName:   declarationNameBase(entry.Declaration),
			})
		}
		names, err := IdentityWITNames(named)
		if err != nil {
			return nil, fmt.Errorf("name declarations in %s: %w", pkg.PkgPath, err)
		}
		namesByPackage[pkg.PkgPath] = names
	}

	exclusions := make(map[string]Exclusion, len(options.Exclusions))
	for _, exclusion := range options.Exclusions {
		exclusions[exclusion.Identity] = exclusion
	}
	matchedExclusions := make(map[string]struct{}, len(exclusions))

	api := &model.API{
		FormatVersion: 1,
		ModulePath:    options.ModulePath,
	}
	lowerer := Lowerer{ModulePath: options.ModulePath}
	for _, analysis := range analyses {
		entries := append(
			slices.Clone(analysis.entries),
			instanceEntries[analysis.pkg.PkgPath]...,
		)
		slices.SortFunc(entries, func(left, right declarationEntry) int {
			return compareDeclarations(left.Declaration, right.Declaration)
		})
		canonicalPackage := model.Package{
			Path:          analysis.pkg.PkgPath,
			Name:          analysis.pkg.Name,
			WITName:       packageNames[analysis.pkg.PkgPath],
			Documentation: analysis.packageDoc,
		}
		for _, entry := range entries {
			canonical, err := lowerDeclaration(
				options.Dir,
				analysis.pkg,
				analysis.documents,
				entry,
				namesByPackage[analysis.pkg.PkgPath][entry.Declaration.Identity],
				lowerer,
				exclusions,
				matchedExclusions,
			)
			if err != nil {
				return nil, err
			}
			canonicalPackage.Declarations = append(
				canonicalPackage.Declarations,
				canonical.Identity,
			)
			api.Declarations = append(api.Declarations, canonical)
		}
		api.Packages = append(api.Packages, canonicalPackage)
	}
	addEventSubscriptions(api)
	for _, exclusion := range options.Exclusions {
		if _, matched := matchedExclusions[exclusion.Identity]; !matched {
			return nil, fmt.Errorf(
				"public API exclusion %q did not match a public declaration",
				exclusion.Identity,
			)
		}
	}
	if err := api.Normalize(); err != nil {
		return nil, fmt.Errorf("normalize canonical Gate API: %w", err)
	}
	return api, nil
}

func addEventSubscriptions(api *model.API) {
	packageIndex := make(map[string]int, len(api.Packages))
	for index, pkg := range api.Packages {
		packageIndex[pkg.Path] = index
	}
	events := slices.Clone(api.Declarations)
	for _, event := range events {
		if !event.Event ||
			event.Coverage.State != model.CoverageRepresented ||
			event.Type == nil {
			continue
		}
		eventParameter := *event.Type
		eventParameter.GoType = "*" + event.Identity
		resourceIdentity := event.Identity
		resourceWITName := event.WITName
		if event.Type.Kind != model.TypeResource &&
			event.Type.Kind != model.TypeCallback &&
			event.Type.Kind != model.TypeDynamic {
			resourceIdentity += "#pointer"
			resourceWITName += "-pointer"
		}
		eventParameter.Identity = resourceIdentity
		eventParameter.WITName = resourceWITName
		eventParameter.Kind = model.TypeResource
		eventParameter.Ownership = model.OwnershipBorrow
		eventParameter.Lifetime = model.LifetimeBorrowedEvent
		eventParameter.Nullable = false
		eventParameter.ResourceType = resourceIdentity

		handlerIdentity := event.Identity + "#wasm-handler"
		handlerCallable := model.Callable{
			Parameters: []model.Parameter{{
				GoName: "event", WITName: "event", Type: eventParameter,
			}},
			Error: &model.ErrorBehavior{Fallback: true},
		}
		handler := model.Type{
			Identity:     handlerIdentity,
			WITName:      event.WITName + "-handler",
			GoType:       "func(*" + event.Identity + ") error",
			Kind:         model.TypeCallback,
			Ownership:    model.OwnershipOwn,
			Lifetime:     model.LifetimePlugin,
			ResourceType: handlerIdentity,
			Callback: &model.Callback{
				Identity: handlerIdentity, Direction: model.CallbackHostToGuest,
				Callable: handlerCallable, Retained: true, Reentrant: true,
			},
		}
		unsubscribeIdentity := "callback-" + identityHash("func()")
		unsubscribeCallable := model.Callable{}
		unsubscribe := model.Type{
			Identity:     unsubscribeIdentity,
			WITName:      unsubscribeIdentity,
			GoType:       "func()",
			Kind:         model.TypeCallback,
			Ownership:    model.OwnershipOwn,
			Lifetime:     model.LifetimePlugin,
			ResourceType: unsubscribeIdentity,
			Callback: &model.Callback{
				Identity: unsubscribeIdentity, Direction: model.CallbackHostToGuest,
				Callable: unsubscribeCallable, Retained: true, Reentrant: true,
			},
		}
		subscription := model.Declaration{
			Identity:    event.Identity + "#wasm-subscribe",
			PackagePath: event.PackagePath,
			GoName:      "Subscribe" + event.GoName,
			WITName:     "subscribe-" + event.WITName,
			Kind:        model.DeclarationFunction,
			Documentation: "Subscribes a component callback to " + event.GoName +
				" with transactional event mutation.",
			Source: event.Source,
			Callable: &model.Callable{
				Parameters: []model.Parameter{
					{
						GoName: "priority", WITName: "priority",
						Type: model.Type{
							GoType: "int", Kind: model.TypeS64,
							Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
						},
					},
					{GoName: "handler", WITName: "handler", Type: handler},
				},
				Results: []model.Parameter{{
					GoName: "unsubscribe", WITName: "unsubscribe", Type: unsubscribe,
				}},
				Error: &model.ErrorBehavior{Fallback: true},
			},
			Coverage: model.Coverage{State: model.CoverageRepresented},
		}
		subscription.Dependencies = declarationDependencies(subscription)
		api.Declarations = append(api.Declarations, subscription)
		index := packageIndex[event.PackagePath]
		api.Packages[index].Declarations = append(
			api.Packages[index].Declarations,
			subscription.Identity,
		)
	}
}

func lowerDeclaration(
	root string,
	pkg *packages.Package,
	documents map[types.Object]string,
	entry declarationEntry,
	witName string,
	lowerer Lowerer,
	exclusions map[string]Exclusion,
	matchedExclusions map[string]struct{},
) (model.Declaration, error) {
	declaration := model.Declaration{
		Identity:      entry.Declaration.Identity,
		PackagePath:   entry.Declaration.PackagePath,
		GoName:        entry.Declaration.Name,
		WITName:       witName,
		Kind:          canonicalDeclarationKind(entry.Declaration.Kind),
		Documentation: documents[entry.Object],
		Source:        objectSource(root, pkg, entry.Object),
		Event: (entry.Declaration.Kind == DeclarationType ||
			entry.Declaration.Kind == DeclarationAlias) &&
			strings.HasSuffix(entry.Declaration.Name, "Event"),
		Coverage: model.Coverage{State: model.CoverageRepresented},
	}
	if entry.Declaration.Kind == DeclarationMethod {
		declaration.Receiver = &model.Receiver{
			TypeIdentity: entry.Declaration.PackagePath + "." + entry.Declaration.Receiver,
			Pointer:      entry.Declaration.PointerReceiver,
			Promoted:     entry.Selection != nil && len(entry.Selection.Index()) > 1,
		}
	}
	if exclusion, excluded := exclusions[declaration.Identity]; excluded {
		matchedExclusions[declaration.Identity] = struct{}{}
		declaration.Coverage = model.Coverage{
			State:  model.CoverageExcluded,
			Reason: exclusion.Reason,
		}
		return declaration, nil
	}

	state := lowerState{
		lowerer:  lowerer,
		visiting: make(map[string]bool),
	}
	switch object := entry.Object.(type) {
	case *types.Const:
		typ, err := state.lower(entry.Type)
		if err != nil {
			return model.Declaration{}, declarationError(declaration, err)
		}
		declaration.Type = &typ
		declaration.Constant = &model.Constant{
			ExactValue: object.Val().ExactString(),
		}
	case *types.Var:
		typ, err := state.lower(entry.Type)
		if err != nil {
			return model.Declaration{}, declarationError(declaration, err)
		}
		declaration.Type = &typ
		declaration.Variable = &model.Variable{Readable: true, Writable: true}
	case *types.TypeName:
		typ, err := state.lower(entry.Type)
		if err != nil {
			return model.Declaration{}, declarationError(declaration, err)
		}
		declaration.Type = &typ
	case *types.Func:
		signature, ok := entry.Type.(*types.Signature)
		if !ok {
			return model.Declaration{}, declarationError(
				declaration,
				fmt.Errorf("resolved function has type %T", entry.Type),
			)
		}
		callable, err := state.lowerCallable(signature)
		if err != nil {
			return model.Declaration{}, declarationError(declaration, err)
		}
		declaration.Callable = &callable
	default:
		return model.Declaration{}, declarationError(
			declaration,
			fmt.Errorf("unsupported declaration object %T", entry.Object),
		)
	}
	declaration.Dependencies = declarationDependencies(declaration)
	return declaration, nil
}

func canonicalDeclarationKind(kind DeclarationKind) model.DeclarationKind {
	switch kind {
	case DeclarationAlias:
		return model.DeclarationAlias
	case DeclarationConstant:
		return model.DeclarationConstant
	case DeclarationFunction:
		return model.DeclarationFunction
	case DeclarationMethod:
		return model.DeclarationMethod
	case DeclarationType:
		return model.DeclarationType
	case DeclarationVariable:
		return model.DeclarationVariable
	default:
		return model.DeclarationKind(kind)
	}
}

func declarationNameBase(declaration Declaration) string {
	if declaration.Receiver != "" {
		return declaration.Receiver + "-" + declaration.Name
	}
	if strings.Contains(declaration.Identity, "[") {
		return declaration.Name + "-" + identityHash(declaration.Identity)
	}
	return declaration.Name
}

func declarationError(declaration model.Declaration, err error) error {
	return fmt.Errorf(
		"lower declaration %s (%s): %w",
		declaration.Identity,
		declaration.Source.File,
		err,
	)
}

func documentationIndex(pkg *packages.Package) (map[types.Object]string, string) {
	documents := make(map[types.Object]string)
	var packageDocuments []string
	for _, file := range pkg.Syntax {
		if file.Doc != nil {
			if text := strings.TrimSpace(file.Doc.Text()); text != "" {
				packageDocuments = append(packageDocuments, text)
			}
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if object := pkg.TypesInfo.Defs[declaration.Name]; object != nil {
					documents[object] = commentText(declaration.Doc)
				}
			case *ast.GenDecl:
				for _, spec := range declaration.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						doc := spec.Doc
						if doc == nil {
							doc = declaration.Doc
						}
						if object := pkg.TypesInfo.Defs[spec.Name]; object != nil {
							documents[object] = commentText(doc)
						}
					case *ast.ValueSpec:
						doc := spec.Doc
						if doc == nil {
							doc = declaration.Doc
						}
						for _, name := range spec.Names {
							if object := pkg.TypesInfo.Defs[name]; object != nil {
								documents[object] = commentText(doc)
							}
						}
					}
				}
			}
		}
	}
	slices.Sort(packageDocuments)
	return documents, strings.Join(slices.Compact(packageDocuments), "\n\n")
}

func commentText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.Text())
}

func objectSource(root string, pkg *packages.Package, object types.Object) model.Source {
	if object == nil || pkg.Fset == nil {
		return model.Source{}
	}
	position := pkg.Fset.Position(object.Pos())
	file := position.Filename
	if relative, err := filepath.Rel(root, file); err == nil {
		file = relative
	}
	return model.Source{
		File:   filepath.ToSlash(file),
		Line:   position.Line,
		Column: position.Column,
	}
}

func declarationDependencies(declaration model.Declaration) []string {
	dependencies := make(map[string]struct{})
	var visitCallable func(model.Callable)
	var visitType func(model.Type)
	visitCallable = func(callable model.Callable) {
		for _, parameter := range callable.Parameters {
			visitType(parameter.Type)
		}
		for _, result := range callable.Results {
			visitType(result.Type)
		}
		if callable.Error != nil && callable.Error.TypedErrorIdentity != "" {
			dependencies[callable.Error.TypedErrorIdentity] = struct{}{}
		}
	}
	visitType = func(typ model.Type) {
		if typ.Identity != "" {
			dependencies[typ.Identity] = struct{}{}
		}
		if typ.Element != nil {
			visitType(*typ.Element)
		}
		if typ.Key != nil {
			visitType(*typ.Key)
		}
		for _, field := range typ.Fields {
			visitType(field.Type)
		}
		for _, variant := range typ.Cases {
			if variant.Type != nil {
				visitType(*variant.Type)
			}
		}
		for _, tuple := range typ.Tuple {
			visitType(tuple)
		}
		if typ.Callback != nil {
			visitCallable(typ.Callback.Callable)
		}
	}
	if declaration.Type != nil {
		visitType(*declaration.Type)
	}
	if declaration.Callable != nil {
		visitCallable(*declaration.Callable)
	}
	delete(dependencies, declaration.Identity)
	result := make([]string, 0, len(dependencies))
	for dependency := range dependencies {
		result = append(result, dependency)
	}
	slices.Sort(result)
	return result
}
