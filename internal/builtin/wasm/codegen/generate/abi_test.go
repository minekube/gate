package generate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/builtin/wasm/codegen/model"
)

func TestABILayoutsCoverCanonicalShapes(t *testing.T) {
	tests := []struct {
		name       string
		typ        model.Type
		direction  ABIDirection
		kind       ABIKind
		size       uint64
		alignment  uint32
		allocator  ABIAllocator
		fieldCount int
	}{
		{
			name: "scalar", typ: scalarType("uint32", model.TypeU32),
			direction: ABIInput, kind: ABIKindScalar, size: 4, alignment: 4,
		},
		{
			name: "borrowed string", typ: scalarType("string", model.TypeString),
			direction: ABIInput, kind: ABIKindBuffer, size: 16, alignment: 8,
		},
		{
			name: "owned string", typ: scalarType("string", model.TypeString),
			direction: ABIOutput, kind: ABIKindBuffer, size: 24, alignment: 8,
			allocator: ABIAllocatorRust,
		},
		{
			name: "nullable resource",
			typ: model.Type{
				Identity: "fixture.Resource", GoType: "*fixture.Resource",
				Kind: model.TypeResource, Nullable: true,
				Ownership: model.OwnershipBorrow, Lifetime: model.LifetimeGateOwned,
			},
			direction: ABIInput, kind: ABIKindOption, size: 16, alignment: 8,
		},
		{
			name: "record",
			typ: model.Type{
				GoType: "fixture.Record", Kind: model.TypeRecord,
				Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
				Fields: []model.Field{
					{GoName: "Flag", WITName: "flag", Type: scalarType("bool", model.TypeBool)},
					{GoName: "Count", WITName: "count", Type: scalarType("uint64", model.TypeU64)},
				},
			},
			direction: ABIInput, kind: ABIKindRecord, size: 16, alignment: 8,
			fieldCount: 2,
		},
		{
			name: "nested list",
			typ: model.Type{
				GoType: "[][]string", Kind: model.TypeList,
				Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
				Element: typePointer(model.Type{
					GoType: "[]string", Kind: model.TypeList,
					Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
					Element: typePointer(scalarType("string", model.TypeString)),
				}),
			},
			direction: ABIOutput, kind: ABIKindBuffer, size: 24, alignment: 8,
			allocator: ABIAllocatorRust,
		},
		{
			name: "callback",
			typ: model.Type{
				Identity: "fixture.Callback", GoType: "func()",
				Kind: model.TypeCallback, Ownership: model.OwnershipOwn,
				Lifetime: model.LifetimePlugin,
			},
			direction: ABIInput, kind: ABIKindHandle, size: 8, alignment: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, err := LayoutType(tt.typ, tt.direction)
			require.NoError(t, err)
			require.Equal(t, tt.kind, layout.Kind)
			require.Equal(t, tt.size, layout.Size)
			require.Equal(t, tt.alignment, layout.Alignment)
			require.Equal(t, tt.allocator, layout.Allocator)
			if tt.fieldCount != 0 {
				require.Len(t, layout.Fields, tt.fieldCount)
			}
		})
	}
}

func TestABIUsesExplicitDiscriminantsAndStableOffsets(t *testing.T) {
	variant := model.Type{
		GoType: "fixture.Variant", Kind: model.TypeVariant,
		Ownership: model.OwnershipCopy, Lifetime: model.LifetimeValue,
		Cases: []model.Case{
			{GoName: "Empty", WITName: "empty"},
			{
				GoName: "Text", WITName: "text",
				Type: typePointer(scalarType("string", model.TypeString)),
			},
		},
	}
	layout, err := LayoutType(variant, ABIInput)
	require.NoError(t, err)
	require.Equal(t, ABIKindVariant, layout.Kind)
	require.EqualValues(t, 32, layout.DiscriminantBits)
	require.EqualValues(t, 8, layout.PayloadOffset)
	require.EqualValues(t, 24, layout.Size)

	option := scalarType("string", model.TypeString)
	option.Nullable = true
	layout, err = LayoutType(option, ABIInput)
	require.NoError(t, err)
	require.Equal(t, ABIKindOption, layout.Kind)
	require.EqualValues(t, 8, layout.DiscriminantBits)
	require.EqualValues(t, 8, layout.PayloadOffset)
	require.EqualValues(t, 24, layout.Size)
}

func TestABISchemaAndFingerprintAreDeterministic(t *testing.T) {
	api := simpleAPI()
	schema, err := BuildABI(api)
	require.NoError(t, err)
	require.EqualValues(t, 1, schema.Version)
	require.NotEmpty(t, schema.Layouts)
	require.Len(t, schema.Fingerprint, 64)

	shuffled := cloneAPI(t, api)
	for left, right := 0, len(shuffled.Declarations)-1; left < right; left, right = left+1, right-1 {
		shuffled.Declarations[left], shuffled.Declarations[right] =
			shuffled.Declarations[right], shuffled.Declarations[left]
	}
	again, err := BuildABI(shuffled)
	require.NoError(t, err)
	require.Equal(t, schema, again)
}

func TestABIRejectsImplicitOrArchitectureSizedKinds(t *testing.T) {
	_, err := LayoutType(model.Type{
		GoType: "uintptr", Kind: "uintptr",
	}, ABIInput)
	require.ErrorContains(t, err, "unsupported ABI type kind")
}
