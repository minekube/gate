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
	addRuntimeExtensions(api)
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

func addRuntimeExtensions(api *model.API) {
	commandPackage := api.ModulePath + "/pkg/command"
	gatePackage := api.ModulePath + "/pkg/gate"
	if !hasCanonicalPackage(api, commandPackage) ||
		!hasCanonicalPackage(api, gatePackage) {
		return
	}
	unsubscribe := syntheticCallback(
		"callback-"+identityHash("func()"),
		"func()",
		model.Callable{},
	)
	commandHandler := syntheticCallback(
		"callback-"+identityHash(
			"func(c *go.minekube.com/gate/pkg/command.Context) error",
		),
		"func(c *go.minekube.com/gate/pkg/command.Context) error",
		model.Callable{
			Parameters: []model.Parameter{{
				GoName: "c", WITName: "c",
				Type: model.Type{
					Identity: "go.minekube.com/gate/pkg/command.Context#pointer",
					WITName:  "context-pointer",
					GoType:   "*go.minekube.com/gate/pkg/command.Context",
					Kind:     model.TypeResource, Ownership: model.OwnershipBorrow,
					Lifetime: model.LifetimeBorrowedCall, Nullable: true,
					ResourceType: "go.minekube.com/gate/pkg/command.Context#pointer",
				},
			}},
			Error: &model.ErrorBehavior{Fallback: true},
		},
	)
	timerHandler := syntheticCallback(
		"callback-"+identityHash("func() error"),
		"func() error",
		model.Callable{Error: &model.ErrorBehavior{Fallback: true}},
	)
	stringType := model.Type{
		GoType: "string", Kind: model.TypeString,
		Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
	}
	s64Type := model.Type{
		GoType: "int64", Kind: model.TypeS64,
		Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
	}
	stringList := model.Type{
		GoType: "[]string", Kind: model.TypeList,
		Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
		Nullable: true, Element: &stringType,
	}
	contextType := model.Type{
		Identity: "context.Context", WITName: "context",
		GoType: "context.Context", Kind: model.TypeResource,
		Ownership: model.OwnershipBorrow, Lifetime: model.LifetimeBorrowedCall,
		ResourceType: "context.Context",
	}
	boolType := model.Type{
		GoType: "bool", Kind: model.TypeBool,
		Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
	}
	appendSyntheticDeclaration(api, model.Declaration{
		Identity:    commandPackage + "#wasm-register-command",
		PackagePath: commandPackage,
		GoName:      "WasmRegisterCommand",
		WITName:     "wasm-register-command",
		Kind:        model.DeclarationFunction,
		Documentation: "Registers a component command and returns an explicit " +
			"unregister callback.",
		Callable: &model.Callable{
			Parameters: []model.Parameter{
				{GoName: "name", WITName: "name", Type: stringType},
				{GoName: "aliases", WITName: "aliases", Type: stringList},
				{GoName: "execute", WITName: "execute", Type: commandHandler},
			},
			Results: []model.Parameter{{
				GoName: "unregister", WITName: "unregister", Type: unsubscribe,
			}},
			Error: &model.ErrorBehavior{Fallback: true},
		},
		Coverage: model.Coverage{State: model.CoverageRepresented},
	})
	for _, timer := range []struct {
		name, documentation string
	}{
		{"after", "Schedules one component callback after a delay."},
		{"every", "Schedules a non-overlapping recurring component callback."},
	} {
		appendSyntheticDeclaration(api, model.Declaration{
			Identity:      gatePackage + "#wasm-" + timer.name,
			PackagePath:   gatePackage,
			GoName:        "Wasm" + strings.ToUpper(timer.name[:1]) + timer.name[1:],
			WITName:       "wasm-" + timer.name,
			Kind:          model.DeclarationFunction,
			Documentation: timer.documentation,
			Callable: &model.Callable{
				Parameters: []model.Parameter{
					{GoName: "nanoseconds", WITName: "nanoseconds", Type: s64Type},
					{GoName: "handler", WITName: "handler", Type: timerHandler},
				},
				Results: []model.Parameter{{
					GoName: "cancel", WITName: "cancel", Type: unsubscribe,
				}},
				Error: &model.ErrorBehavior{Fallback: true},
			},
			Coverage: model.Coverage{State: model.CoverageRepresented},
		})
	}
	for _, extension := range []struct {
		name, goName, documentation string
		results                     []model.Parameter
	}{
		{
			name: "context-cancelled", goName: "WasmContextCancelled",
			documentation: "Reports whether the plugin context has been cancelled.",
			results: []model.Parameter{{
				GoName: "cancelled", WITName: "cancelled", Type: boolType,
			}},
		},
		{
			name: "context-deadline", goName: "WasmContextDeadline",
			documentation: "Returns the plugin context deadline as Unix nanoseconds.",
			results: []model.Parameter{
				{GoName: "unixNanos", WITName: "unix-nanos", Type: s64Type},
				{GoName: "ok", WITName: "ok", Type: boolType},
			},
		},
		{
			name: "context-error", goName: "WasmContextError",
			documentation: "Returns the plugin context error or an empty string.",
			results: []model.Parameter{{
				GoName: "message", WITName: "message", Type: stringType,
			}},
		},
	} {
		appendSyntheticDeclaration(api, model.Declaration{
			Identity:      gatePackage + "#wasm-" + extension.name,
			PackagePath:   gatePackage,
			GoName:        extension.goName,
			WITName:       "wasm-" + extension.name,
			Kind:          model.DeclarationFunction,
			Documentation: extension.documentation,
			Callable: &model.Callable{
				Parameters: []model.Parameter{{
					GoName: "ctx", WITName: "ctx", Type: contextType,
				}},
				Results: extension.results,
			},
			Coverage: model.Coverage{State: model.CoverageRepresented},
		})
	}
	appendSyntheticDeclaration(api, model.Declaration{
		Identity:    gatePackage + "#wasm-log",
		PackagePath: gatePackage,
		GoName:      "WasmLog",
		WITName:     "wasm-log",
		Kind:        model.DeclarationFunction,
		Documentation: "Logs through the logger carried by the plugin context. " +
			"Fields are alternating key/value strings.",
		Callable: &model.Callable{
			Parameters: []model.Parameter{
				{GoName: "ctx", WITName: "ctx", Type: contextType},
				{GoName: "level", WITName: "level", Type: s64Type},
				{GoName: "message", WITName: "message", Type: stringType},
				{GoName: "fields", WITName: "fields", Type: stringList},
			},
			Error: &model.ErrorBehavior{Fallback: true},
		},
		Coverage: model.Coverage{State: model.CoverageRepresented},
	})
}

func hasCanonicalPackage(api *model.API, path string) bool {
	for _, pkg := range api.Packages {
		if pkg.Path == path {
			return true
		}
	}
	return false
}

func syntheticCallback(identity, goType string, callable model.Callable) model.Type {
	return model.Type{
		Identity: identity, WITName: identity, GoType: goType,
		Kind: model.TypeCallback, Ownership: model.OwnershipOwn,
		Lifetime: model.LifetimePlugin, ResourceType: identity,
		Callback: &model.Callback{
			Identity: identity, Direction: model.CallbackHostToGuest,
			Callable: callable, Retained: true, Reentrant: true,
		},
	}
}

func appendSyntheticDeclaration(api *model.API, declaration model.Declaration) {
	declaration.Dependencies = declarationDependencies(declaration)
	api.Declarations = append(api.Declarations, declaration)
	for index := range api.Packages {
		if api.Packages[index].Path == declaration.PackagePath {
			api.Packages[index].Declarations = append(
				api.Packages[index].Declarations,
				declaration.Identity,
			)
			return
		}
	}
	panic("synthetic declaration package is missing: " + declaration.PackagePath)
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
	if relative, err := filepath.Rel(root, file); err == nil &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		file = relative
	} else if object.Pkg() != nil {
		file = filepath.Join(
			"go",
			filepath.FromSlash(object.Pkg().Path()),
			filepath.Base(file),
		)
	} else {
		file = filepath.Base(file)
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
