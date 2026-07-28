package analyze

import (
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"go.minekube.com/gate/internal/wasm/model"
)

// Lowerer converts resolved Go types into the canonical component model.
type Lowerer struct {
	ModulePath string
}

// LowerType lowers one resolved Go type.
func (l Lowerer) LowerType(goType types.Type) (model.Type, error) {
	if goType == nil {
		return model.Type{}, fmt.Errorf("cannot lower a nil Go type")
	}
	state := lowerState{
		lowerer:  l,
		visiting: make(map[string]bool),
	}
	return state.lower(goType)
}

type lowerState struct {
	lowerer  Lowerer
	visiting map[string]bool
}

type genericInstance struct {
	OriginIdentity   string
	InstanceIdentity string
	Type             types.Type
	TypeArguments    []types.Type
}

func collectGenericInstances(pkgs []*packages.Package) ([]genericInstance, error) {
	instances := make(map[string]genericInstance)
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			return nil, fmt.Errorf("package %q has no types info", pkg.PkgPath)
		}
		idents := make([]*ast.Ident, 0, len(pkg.TypesInfo.Instances))
		for ident := range pkg.TypesInfo.Instances {
			idents = append(idents, ident)
		}
		slices.SortFunc(idents, func(left, right *ast.Ident) int {
			leftPosition := pkg.Fset.Position(left.Pos())
			rightPosition := pkg.Fset.Position(right.Pos())
			if byFile := strings.Compare(leftPosition.Filename, rightPosition.Filename); byFile != 0 {
				return byFile
			}
			return leftPosition.Offset - rightPosition.Offset
		})
		for _, ident := range idents {
			instance := pkg.TypesInfo.Instances[ident]
			object := pkg.TypesInfo.Uses[ident]
			if object == nil {
				object = pkg.TypesInfo.Defs[ident]
			}
			origin := objectIdentity(object)
			if origin == "" {
				return nil, fmt.Errorf(
					"generic instance %q in %s has no resolved origin",
					ident.Name,
					pkg.Fset.Position(ident.Pos()),
				)
			}
			arguments := make([]types.Type, instance.TypeArgs.Len())
			argumentNames := make([]string, instance.TypeArgs.Len())
			for index := range instance.TypeArgs.Len() {
				arguments[index] = instance.TypeArgs.At(index)
				argumentNames[index] = goTypeString(arguments[index])
			}
			identity := origin + "[" + strings.Join(argumentNames, ",") + "]"
			instances[identity] = genericInstance{
				OriginIdentity:   origin,
				InstanceIdentity: identity,
				Type:             instance.Type,
				TypeArguments:    arguments,
			}
		}
	}
	result := make([]genericInstance, 0, len(instances))
	for _, instance := range instances {
		result = append(result, instance)
	}
	slices.SortFunc(result, func(left, right genericInstance) int {
		return strings.Compare(left.InstanceIdentity, right.InstanceIdentity)
	})
	return result, nil
}

func (s *lowerState) lower(goType types.Type) (model.Type, error) {
	if alias, ok := goType.(*types.Alias); ok {
		return s.lower(types.Unalias(alias))
	}
	if named, ok := goType.(*types.Named); ok {
		return s.lowerNamed(named)
	}

	switch goType := goType.(type) {
	case *types.Basic:
		return s.lowerBasic(goType)
	case *types.Array:
		element, err := s.lower(goType.Elem())
		if err != nil {
			return model.Type{}, err
		}
		return model.Type{
			GoType:      goTypeString(goType),
			Kind:        model.TypeList,
			Ownership:   model.OwnershipCopy,
			Lifetime:    model.LifetimeValue,
			Element:     &element,
			ArrayLength: goType.Len(),
		}, nil
	case *types.Slice:
		element, err := s.lower(goType.Elem())
		if err != nil {
			return model.Type{}, err
		}
		return model.Type{
			GoType:    goTypeString(goType),
			Kind:      model.TypeList,
			Ownership: model.OwnershipCopy,
			Lifetime:  model.LifetimeValue,
			Nullable:  true,
			Element:   &element,
		}, nil
	case *types.Map:
		key, err := s.lower(goType.Key())
		if err != nil {
			return model.Type{}, fmt.Errorf("lower map key: %w", err)
		}
		value, err := s.lower(goType.Elem())
		if err != nil {
			return model.Type{}, fmt.Errorf("lower map value: %w", err)
		}
		entry := model.Type{
			GoType:    "map-entry<" + key.GoType + "," + value.GoType + ">",
			Kind:      model.TypeRecord,
			Ownership: model.OwnershipCopy,
			Lifetime:  model.LifetimeValue,
			Fields: []model.Field{
				{GoName: "Key", WITName: "key", Type: key},
				{GoName: "Value", WITName: "value", Type: value},
			},
		}
		return model.Type{
			GoType:    goTypeString(goType),
			Kind:      model.TypeList,
			Ownership: model.OwnershipCopy,
			Lifetime:  model.LifetimeValue,
			Nullable:  true,
			Element:   &entry,
		}, nil
	case *types.Pointer:
		return s.resource(goType, typeIdentity(goType.Elem()), true, model.LifetimeGateOwned), nil
	case *types.Interface:
		if goType.Empty() {
			return model.Type{
				GoType:    goTypeString(goType),
				Kind:      model.TypeDynamic,
				Ownership: model.OwnershipCopy,
				Lifetime:  model.LifetimeValue,
				Nullable:  true,
			}, nil
		}
		return s.resource(goType, "", true, model.LifetimeGateOwned), nil
	case *types.Chan:
		element, err := s.lower(goType.Elem())
		if err != nil {
			return model.Type{}, err
		}
		direction := model.ChannelBidirectional
		switch goType.Dir() {
		case types.SendOnly:
			direction = model.ChannelSend
		case types.RecvOnly:
			direction = model.ChannelReceive
		}
		typ := s.resource(goType, "", true, model.LifetimePlugin)
		typ.Element = &element
		typ.ChannelDirection = direction
		return typ, nil
	case *types.Signature:
		return s.lowerSignatureType(goType, "")
	case *types.Struct:
		return s.lowerStruct(goType, "", "")
	case *types.Tuple:
		tuple := make([]model.Type, goType.Len())
		for index := range goType.Len() {
			element, err := s.lower(goType.At(index).Type())
			if err != nil {
				return model.Type{}, fmt.Errorf("lower tuple item %d: %w", index, err)
			}
			tuple[index] = element
		}
		return model.Type{
			GoType:    goTypeString(goType),
			Kind:      model.TypeTuple,
			Ownership: model.OwnershipCopy,
			Lifetime:  model.LifetimeValue,
			Tuple:     tuple,
		}, nil
	case *types.TypeParam, *types.Union:
		return model.Type{
			GoType:    goTypeString(goType),
			Kind:      model.TypeDynamic,
			Ownership: model.OwnershipCopy,
			Lifetime:  model.LifetimeValue,
			Nullable:  true,
		}, nil
	default:
		return s.resource(goType, "", false, model.LifetimeGateOwned), nil
	}
}

func (s *lowerState) lowerNamed(named *types.Named) (model.Type, error) {
	identity := namedIdentity(named)
	switch identity {
	case "context.Context":
		return model.Type{
			Identity:     identity,
			WITName:      "context",
			GoType:       goTypeString(named),
			Kind:         model.TypeResource,
			Ownership:    model.OwnershipBorrow,
			Lifetime:     model.LifetimeBorrowedCall,
			Nullable:     true,
			ResourceType: identity,
		}, nil
	case "time.Duration":
		return model.Type{
			Identity:  identity,
			WITName:   "duration",
			GoType:    goTypeString(named),
			Kind:      model.TypeS64,
			Ownership: model.OwnershipCopy,
			Lifetime:  model.LifetimeValue,
		}, nil
	case "time.Time":
		return timestampType(goTypeString(named)), nil
	}

	if s.visiting[identity] {
		return s.resource(named, identity, false, model.LifetimeGateOwned), nil
	}
	s.visiting[identity] = true
	defer delete(s.visiting, identity)

	var (
		lowered model.Type
		err     error
	)
	switch underlying := named.Underlying().(type) {
	case *types.Struct:
		lowered, err = s.lowerStruct(underlying, identity, namedWITName(named))
	case *types.Signature:
		lowered, err = s.lowerSignatureType(underlying, identity)
	default:
		lowered, err = s.lower(underlying)
	}
	if err != nil {
		return model.Type{}, fmt.Errorf("lower %s: %w", identity, err)
	}
	lowered.Identity = identity
	lowered.WITName = namedWITName(named)
	lowered.GoType = goTypeString(named)
	if lowered.Kind == model.TypeResource {
		lowered.ResourceType = identity
	}
	return lowered, nil
}

func (s *lowerState) lowerBasic(basic *types.Basic) (model.Type, error) {
	typ := model.Type{
		GoType:    goTypeString(basic),
		Ownership: model.OwnershipCopy,
		Lifetime:  model.LifetimeValue,
	}
	switch basic.Kind() {
	case types.Bool, types.UntypedBool:
		typ.Kind = model.TypeBool
	case types.Int8:
		typ.Kind = model.TypeS8
	case types.Int16:
		typ.Kind = model.TypeS16
	case types.Int32, types.UntypedRune:
		typ.Kind = model.TypeS32
	case types.Int, types.Int64, types.UntypedInt:
		typ.Kind = model.TypeS64
	case types.Uint8:
		typ.Kind = model.TypeU8
	case types.Uint16:
		typ.Kind = model.TypeU16
	case types.Uint32:
		typ.Kind = model.TypeU32
	case types.Uint, types.Uint64, types.Uintptr:
		typ.Kind = model.TypeU64
	case types.Float32:
		typ.Kind = model.TypeF32
	case types.Float64, types.UntypedFloat:
		typ.Kind = model.TypeF64
	case types.String, types.UntypedString:
		typ.Kind = model.TypeString
	case types.Complex64:
		return complexType(goTypeString(basic), model.TypeF32), nil
	case types.Complex128, types.UntypedComplex:
		return complexType(goTypeString(basic), model.TypeF64), nil
	case types.UnsafePointer:
		return s.resource(basic, "unsafe.Pointer", false, model.LifetimeGateOwned), nil
	case types.UntypedNil:
		typ.Kind = model.TypeDynamic
		typ.Nullable = true
	default:
		return model.Type{}, fmt.Errorf("unsupported Go basic type %s", basic)
	}
	return typ, nil
}

func (s *lowerState) lowerStruct(
	structType *types.Struct,
	identity string,
	witName string,
) (model.Type, error) {
	for index := range structType.NumFields() {
		if !structType.Field(index).Exported() {
			return s.resource(structType, identity, false, model.LifetimeGateOwned), nil
		}
	}
	fields := make([]model.Field, structType.NumFields())
	for index := range structType.NumFields() {
		field := structType.Field(index)
		fieldType, err := s.lower(field.Type())
		if err != nil {
			return model.Type{}, fmt.Errorf("field %s: %w", field.Name(), err)
		}
		fields[index] = model.Field{
			GoName:  field.Name(),
			WITName: WITIdentifier(field.Name()),
			Type:    fieldType,
		}
	}
	return model.Type{
		Identity:  identity,
		WITName:   witName,
		GoType:    goTypeString(structType),
		Kind:      model.TypeRecord,
		Ownership: model.OwnershipCopy,
		Lifetime:  model.LifetimeValue,
		Fields:    fields,
	}, nil
}

func (s *lowerState) lowerSignatureType(
	signature *types.Signature,
	identity string,
) (model.Type, error) {
	callable, err := s.lowerCallable(signature)
	if err != nil {
		return model.Type{}, err
	}
	if identity == "" {
		identity = "callback-" + identityHash(goTypeString(signature))
	}
	return model.Type{
		Identity:     identity,
		WITName:      WITIdentifier(shortIdentity(identity)),
		GoType:       goTypeString(signature),
		Kind:         model.TypeCallback,
		Ownership:    model.OwnershipOwn,
		Lifetime:     model.LifetimePlugin,
		Nullable:     true,
		ResourceType: identity,
		Callback: &model.Callback{
			Identity:  identity,
			Direction: model.CallbackHostToGuest,
			Callable:  callable,
			Retained:  true,
			Reentrant: true,
		},
	}, nil
}

func (s *lowerState) lowerCallable(signature *types.Signature) (model.Callable, error) {
	parameters, err := s.lowerTuple(signature.Params(), "arg")
	if err != nil {
		return model.Callable{}, err
	}
	results := signature.Results()
	callable := model.Callable{
		Parameters: parameters,
		Variadic:   signature.Variadic(),
	}
	resultLimit := results.Len()
	if resultLimit > 0 && isErrorType(results.At(resultLimit-1).Type()) {
		errorType := results.At(resultLimit - 1).Type()
		callable.Error = &model.ErrorBehavior{Fallback: true}
		if identity := typeIdentity(errorType); identity != "" && identity != "error" {
			callable.Error.TypedErrorIdentity = identity
		}
		resultLimit--
	}
	callable.Results, err = s.lowerTuplePrefix(results, "result", resultLimit)
	return callable, err
}

func (s *lowerState) lowerTuple(tuple *types.Tuple, fallback string) ([]model.Parameter, error) {
	return s.lowerTuplePrefix(tuple, fallback, tuple.Len())
}

func (s *lowerState) lowerTuplePrefix(
	tuple *types.Tuple,
	fallback string,
	limit int,
) ([]model.Parameter, error) {
	parameters := make([]model.Parameter, limit)
	for index := range limit {
		variable := tuple.At(index)
		name := variable.Name()
		if name == "" {
			name = fallback + strconv.Itoa(index)
		}
		parameterType, err := s.lower(variable.Type())
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", fallback, name, err)
		}
		parameters[index] = model.Parameter{
			GoName:  name,
			WITName: WITIdentifier(name),
			Type:    parameterType,
		}
	}
	return parameters, nil
}

func (s *lowerState) resource(
	goType types.Type,
	identity string,
	nullable bool,
	lifetime model.Lifetime,
) model.Type {
	if identity == "" {
		identity = typeIdentity(goType)
	}
	if identity == "" {
		identity = "opaque-" + identityHash(goTypeString(goType))
	}
	return model.Type{
		Identity:     identity,
		WITName:      WITIdentifier(shortIdentity(identity)),
		GoType:       goTypeString(goType),
		Kind:         model.TypeResource,
		Ownership:    model.OwnershipBorrow,
		Lifetime:     lifetime,
		Nullable:     nullable,
		ResourceType: identity,
	}
}

func complexType(goType string, part model.TypeKind) model.Type {
	component := model.Type{
		GoType:    string(part),
		Kind:      part,
		Ownership: model.OwnershipCopy,
		Lifetime:  model.LifetimeValue,
	}
	return model.Type{
		GoType:    goType,
		Kind:      model.TypeRecord,
		Ownership: model.OwnershipCopy,
		Lifetime:  model.LifetimeValue,
		Fields: []model.Field{
			{GoName: "Real", WITName: "real", Type: component},
			{GoName: "Imaginary", WITName: "imaginary", Type: component},
		},
	}
}

func timestampType(goType string) model.Type {
	return model.Type{
		Identity:  "time.Time",
		WITName:   "timestamp",
		GoType:    goType,
		Kind:      model.TypeRecord,
		Ownership: model.OwnershipCopy,
		Lifetime:  model.LifetimeValue,
		Fields: []model.Field{
			{
				GoName:  "Seconds",
				WITName: "seconds",
				Type: model.Type{
					GoType: "int64", Kind: model.TypeS64,
					Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
				},
			},
			{
				GoName:  "Nanoseconds",
				WITName: "nanoseconds",
				Type: model.Type{
					GoType: "uint32", Kind: model.TypeU32,
					Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
				},
			},
		},
	}
}

func namedIdentity(named *types.Named) string {
	identity := typeObjectIdentity(named.Obj())
	if named.TypeArgs() == nil || named.TypeArgs().Len() == 0 {
		return identity
	}
	arguments := make([]string, named.TypeArgs().Len())
	for index := range named.TypeArgs().Len() {
		arguments[index] = goTypeString(named.TypeArgs().At(index))
	}
	return identity + "[" + strings.Join(arguments, ",") + "]"
}

func namedWITName(named *types.Named) string {
	name := WITIdentifier(named.Obj().Name())
	if named.TypeArgs() != nil && named.TypeArgs().Len() > 0 {
		name += "-" + identityHash(namedIdentity(named))
	}
	return name
}

func typeIdentity(goType types.Type) string {
	switch goType := goType.(type) {
	case *types.Alias:
		return typeIdentity(types.Unalias(goType))
	case *types.Named:
		return namedIdentity(goType)
	case *types.Pointer:
		return typeIdentity(goType.Elem())
	case *types.Interface:
		if goType == types.Universe.Lookup("error").Type().Underlying() {
			return "error"
		}
	}
	return ""
}

func typeObjectIdentity(object *types.TypeName) string {
	if object == nil {
		return ""
	}
	if object.Pkg() == nil {
		return object.Name()
	}
	return object.Pkg().Path() + "." + object.Name()
}

func objectIdentity(object types.Object) string {
	if object == nil {
		return ""
	}
	if object.Pkg() == nil {
		return object.Name()
	}
	return object.Pkg().Path() + "." + object.Name()
}

func isErrorType(goType types.Type) bool {
	errorObject := types.Universe.Lookup("error")
	errorInterface, ok := errorObject.Type().Underlying().(*types.Interface)
	if !ok {
		return false
	}
	return types.AssignableTo(goType, errorObject.Type()) ||
		types.Implements(goType, errorInterface)
}

func goTypeString(goType types.Type) string {
	return types.TypeString(goType, func(pkg *types.Package) string {
		return pkg.Path()
	})
}

func shortIdentity(identity string) string {
	if dot := strings.LastIndexByte(identity, '.'); dot >= 0 {
		return identity[dot+1:]
	}
	return identity
}
