package analyze

import (
	"go/types"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"go.minekube.com/gate/internal/wasm/model"
)

func TestLowerRepresentativeGoTypes(t *testing.T) {
	pkg := loadShapeFixture(t)
	lowerer := Lowerer{ModulePath: "example.com/fixture"}

	tests := []struct {
		name     string
		kind     model.TypeKind
		nullable bool
		lifetime model.Lifetime
		check    func(*testing.T, model.Type)
	}{
		{name: "Bool", kind: model.TypeBool},
		{name: "Int", kind: model.TypeS64},
		{name: "Uintptr", kind: model.TypeU64},
		{name: "String", kind: model.TypeString},
		{name: "Bytes", kind: model.TypeList, nullable: true, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, model.TypeU8, typ.Element.Kind)
		}},
		{name: "Array", kind: model.TypeList, check: func(t *testing.T, typ model.Type) {
			require.EqualValues(t, 3, typ.ArrayLength)
			require.Equal(t, model.TypeS16, typ.Element.Kind)
		}},
		{name: "Map", kind: model.TypeList, nullable: true, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, model.TypeRecord, typ.Element.Kind)
			require.Equal(t, []string{"key", "value"}, []string{
				typ.Element.Fields[0].WITName,
				typ.Element.Fields[1].WITName,
			})
		}},
		{name: "Value", kind: model.TypeRecord, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, "example.com/fixture/pkg/shapes.Value", typ.Identity)
			require.Equal(t, []string{"text", "values"}, []string{
				typ.Fields[0].WITName,
				typ.Fields[1].WITName,
			})
		}},
		{name: "Hidden", kind: model.TypeResource, lifetime: model.LifetimeGateOwned},
		{name: "Pointer", kind: model.TypeResource, nullable: true, lifetime: model.LifetimeGateOwned},
		{name: "Interface", kind: model.TypeResource, nullable: true, lifetime: model.LifetimeGateOwned},
		{name: "Any", kind: model.TypeDynamic, nullable: true},
		{name: "Channel", kind: model.TypeResource, nullable: true, lifetime: model.LifetimePlugin, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, model.TypeString, typ.Element.Kind)
			require.Equal(t, model.ChannelSend, typ.ChannelDirection)
		}},
		{name: "Callback", kind: model.TypeCallback, nullable: true, lifetime: model.LifetimePlugin, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, model.CallbackHostToGuest, typ.Callback.Direction)
			require.Len(t, typ.Callback.Callable.Parameters, 1)
			require.Len(t, typ.Callback.Callable.Results, 1)
			require.NotNil(t, typ.Callback.Callable.Error)
			require.True(t, typ.Callback.Callable.Error.Fallback)
		}},
		{name: "Duration", kind: model.TypeS64},
		{name: "Timestamp", kind: model.TypeRecord, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, []string{"seconds", "nanoseconds"}, []string{
				typ.Fields[0].WITName,
				typ.Fields[1].WITName,
			})
		}},
		{name: "Context", kind: model.TypeResource, nullable: true, lifetime: model.LifetimeBorrowedCall},
		{name: "Complex", kind: model.TypeRecord, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, []string{"real", "imaginary"}, []string{
				typ.Fields[0].WITName,
				typ.Fields[1].WITName,
			})
		}},
		{name: "Unsafe", kind: model.TypeResource, lifetime: model.LifetimeGateOwned},
		{name: "Recursive", kind: model.TypeRecord, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, model.TypeResource, typ.Fields[1].Type.Kind)
		}},
		{name: "GenericValue", kind: model.TypeRecord, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, model.TypeString, typ.Fields[0].Type.Kind)
			require.Contains(t, typ.Identity, "[string]")
		}},
		{name: "GenericDefinition", kind: model.TypeRecord, check: func(t *testing.T, typ model.Type) {
			require.Equal(t, model.TypeDynamic, typ.Fields[0].Type.Kind)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goType := fixtureType(t, pkg, tt.name)
			lowered, err := lowerer.LowerType(goType)
			require.NoError(t, err)
			require.Equal(t, tt.kind, lowered.Kind)
			require.Equal(t, tt.nullable, lowered.Nullable)
			if tt.lifetime != "" {
				require.Equal(t, tt.lifetime, lowered.Lifetime)
			}
			if tt.check != nil {
				tt.check(t, lowered)
			}
		})
	}
}

func TestLowerAliasesAndNamedScalarsPreserveIdentity(t *testing.T) {
	pkg := loadShapeFixture(t)
	lowerer := Lowerer{ModulePath: "example.com/fixture"}

	alias, err := lowerer.LowerType(fixtureType(t, pkg, "Alias"))
	require.NoError(t, err)
	require.Equal(t, model.TypeRecord, alias.Kind)
	require.Equal(t, "example.com/fixture/pkg/shapes.Value", alias.Identity)

	scalar, err := lowerer.LowerType(fixtureType(t, pkg, "NamedScalar"))
	require.NoError(t, err)
	require.Equal(t, model.TypeS32, scalar.Kind)
	require.Equal(t, "example.com/fixture/pkg/shapes.NamedScalar", scalar.Identity)
	require.Equal(t, "named-scalar", scalar.WITName)
}

func TestGenericInstancesAreResolvedAndDeterministic(t *testing.T) {
	pkg := loadShapeFixture(t)

	instances, err := collectGenericInstances([]*packages.Package{pkg})
	require.NoError(t, err)
	identities := make([]string, len(instances))
	for index, instance := range instances {
		identities[index] = instance.InstanceIdentity
		require.NotNil(t, instance.Type)
		require.NotEmpty(t, instance.TypeArguments)
	}
	require.Equal(t, []string{
		"example.com/fixture/pkg/shapes.Generic[any]",
		"example.com/fixture/pkg/shapes.Generic[string]",
		"example.com/fixture/pkg/shapes.Identity[int32]",
	}, identities)
}

func loadShapeFixture(t *testing.T) *packages.Package {
	t.Helper()
	loaded, err := packages.Load(&packages.Config{
		Dir: filepath.Join("testdata", "module"),
		Mode: packages.NeedName |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedSyntax,
	}, "./pkg/shapes")
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Empty(t, loaded[0].Errors)
	return loaded[0]
}

func fixtureType(t *testing.T, pkg *packages.Package, name string) types.Type {
	t.Helper()
	object := pkg.Types.Scope().Lookup(name)
	require.NotNil(t, object, name)
	switch object := object.(type) {
	case *types.TypeName:
		return object.Type()
	case *types.Var:
		return object.Type()
	default:
		t.Fatalf("%s has unexpected object type %T", name, object)
		return nil
	}
}
