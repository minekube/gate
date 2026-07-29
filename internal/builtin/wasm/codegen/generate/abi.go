package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"go.minekube.com/gate/internal/builtin/wasm/codegen/model"
)

const ABISchemaVersion uint32 = 1

type ABIDirection string

const (
	ABIInput    ABIDirection = "borrowed-input"
	ABIOutput   ABIDirection = "rust-owned-output"
	ABIGoOutput ABIDirection = "go-owned-output"
)

type ABIKind string

const (
	ABIKindScalar  ABIKind = "scalar"
	ABIKindHandle  ABIKind = "handle"
	ABIKindBuffer  ABIKind = "buffer"
	ABIKindRecord  ABIKind = "record"
	ABIKindOption  ABIKind = "option"
	ABIKindVariant ABIKind = "variant"
	ABIKindResult  ABIKind = "result"
	ABIKindTuple   ABIKind = "tuple"
)

type ABIAllocator string

const (
	ABIAllocatorNone ABIAllocator = ""
	ABIAllocatorGo   ABIAllocator = "go"
	ABIAllocatorRust ABIAllocator = "rust"
)

type ABILayout struct {
	Identity         string         `json:"identity,omitempty"`
	GoType           string         `json:"goType,omitempty"`
	Kind             ABIKind        `json:"kind"`
	Scalar           model.TypeKind `json:"scalar,omitempty"`
	Size             uint64         `json:"size"`
	Alignment        uint32         `json:"alignment"`
	Direction        ABIDirection   `json:"direction"`
	Allocator        ABIAllocator   `json:"allocator,omitempty"`
	FreeOperation    string         `json:"freeOperation,omitempty"`
	DiscriminantBits uint32         `json:"discriminantBits,omitempty"`
	PayloadOffset    uint64         `json:"payloadOffset,omitempty"`
	Element          *ABILayout     `json:"element,omitempty"`
	Fields           []ABIField     `json:"fields,omitempty"`
	Cases            []ABICase      `json:"cases,omitempty"`
}

type ABIField struct {
	Name   string    `json:"name"`
	Offset uint64    `json:"offset"`
	Layout ABILayout `json:"layout"`
}

type ABICase struct {
	Name   string     `json:"name"`
	Layout *ABILayout `json:"layout,omitempty"`
}

type ABIEntry struct {
	Identity  string       `json:"identity"`
	Direction ABIDirection `json:"direction"`
	Layout    ABILayout    `json:"layout"`
}

type ABISchema struct {
	Version     uint32     `json:"version"`
	PointerBits uint32     `json:"pointerBits"`
	Layouts     []ABIEntry `json:"layouts"`
	Fingerprint string     `json:"fingerprint"`
}

func LayoutType(typ model.Type, direction ABIDirection) (ABILayout, error) {
	if direction != ABIInput && direction != ABIOutput && direction != ABIGoOutput {
		return ABILayout{}, fmt.Errorf("unsupported ABI direction %q", direction)
	}
	layout, err := layoutCore(typ, direction)
	if err != nil {
		return ABILayout{}, err
	}
	layout.Identity = typ.Identity
	layout.GoType = typ.GoType
	if typ.Nullable && typ.Kind != model.TypeOption {
		layout = optionLayout(layout, direction)
		layout.Identity = typ.Identity
		layout.GoType = typ.GoType
	}
	return layout, nil
}

func layoutCore(typ model.Type, direction ABIDirection) (ABILayout, error) {
	scalar := func(size uint64, alignment uint32) ABILayout {
		return ABILayout{
			Kind: ABIKindScalar, Scalar: typ.Kind,
			Size: size, Alignment: alignment, Direction: direction,
		}
	}
	switch typ.Kind {
	case model.TypeBool, model.TypeS8, model.TypeU8:
		return scalar(1, 1), nil
	case model.TypeS16, model.TypeU16:
		return scalar(2, 2), nil
	case model.TypeS32, model.TypeU32, model.TypeF32, model.TypeChar,
		model.TypeEnum:
		return scalar(4, 4), nil
	case model.TypeS64, model.TypeU64, model.TypeF64, model.TypeFlags:
		return scalar(8, 8), nil
	case model.TypeResource, model.TypeCallback, model.TypeDynamic:
		return ABILayout{
			Kind: ABIKindHandle, Size: 8, Alignment: 8,
			Direction: direction,
		}, nil
	case model.TypeString:
		return bufferLayout(nil, direction), nil
	case model.TypeList:
		if typ.Element == nil {
			return ABILayout{}, fmt.Errorf("%s list has no element", typePath(typ))
		}
		element, err := LayoutType(*typ.Element, direction)
		if err != nil {
			return ABILayout{}, fmt.Errorf("%s list element: %w", typePath(typ), err)
		}
		return bufferLayout(&element, direction), nil
	case model.TypeRecord:
		return recordLayout(typ.Fields, direction, ABIKindRecord)
	case model.TypeTuple:
		fields := make([]model.Field, len(typ.Tuple))
		for index, item := range typ.Tuple {
			fields[index] = model.Field{
				WITName: fmt.Sprintf("item-%d", index),
				Type:    item,
			}
		}
		return recordLayout(fields, direction, ABIKindTuple)
	case model.TypeOption:
		if typ.Element == nil {
			return ABILayout{}, fmt.Errorf("%s option has no element", typePath(typ))
		}
		element, err := LayoutType(*typ.Element, direction)
		if err != nil {
			return ABILayout{}, fmt.Errorf("%s option element: %w", typePath(typ), err)
		}
		return optionLayout(element, direction), nil
	case model.TypeVariant:
		return variantLayout(typ, direction)
	case model.TypeResult:
		return resultLayout(typ, direction)
	default:
		return ABILayout{}, fmt.Errorf(
			"%s has unsupported ABI type kind %q",
			typePath(typ),
			typ.Kind,
		)
	}
}

func bufferLayout(element *ABILayout, direction ABIDirection) ABILayout {
	layout := ABILayout{
		Kind: ABIKindBuffer, Size: 16, Alignment: 8,
		Direction: direction, Element: element,
	}
	switch direction {
	case ABIOutput:
		layout.Size = 24
		layout.Allocator = ABIAllocatorRust
		layout.FreeOperation = "gate_wasm_rust_buffer_free"
	case ABIGoOutput:
		layout.Size = 24
		layout.Allocator = ABIAllocatorGo
		layout.FreeOperation = "gate_wasm_go_buffer_free"
	}
	return layout
}

func recordLayout(
	fields []model.Field,
	direction ABIDirection,
	kind ABIKind,
) (ABILayout, error) {
	layout := ABILayout{
		Kind: kind, Size: 1, Alignment: 1, Direction: direction,
		Fields: make([]ABIField, 0, len(fields)),
	}
	var offset uint64
	for _, field := range fields {
		child, err := LayoutType(field.Type, direction)
		if err != nil {
			return ABILayout{}, fmt.Errorf("field %s: %w", field.WITName, err)
		}
		offset = alignUp(offset, child.Alignment)
		layout.Fields = append(layout.Fields, ABIField{
			Name: field.WITName, Offset: offset, Layout: child,
		})
		offset += child.Size
		layout.Alignment = max(layout.Alignment, child.Alignment)
	}
	if len(fields) != 0 {
		layout.Size = alignUp(offset, layout.Alignment)
	}
	return layout, nil
}

func optionLayout(element ABILayout, direction ABIDirection) ABILayout {
	payloadOffset := alignUp(1, element.Alignment)
	return ABILayout{
		Kind: ABIKindOption, Size: alignUp(payloadOffset+element.Size, element.Alignment),
		Alignment: max(uint32(1), element.Alignment), Direction: direction,
		DiscriminantBits: 8, PayloadOffset: payloadOffset, Element: &element,
	}
}

func variantLayout(typ model.Type, direction ABIDirection) (ABILayout, error) {
	layout := ABILayout{
		Kind: ABIKindVariant, Size: 4, Alignment: 4, Direction: direction,
		DiscriminantBits: 32, Cases: make([]ABICase, 0, len(typ.Cases)),
	}
	var payloadSize uint64
	var payloadAlignment uint32 = 1
	for _, variant := range typ.Cases {
		abiCase := ABICase{Name: variant.WITName}
		if variant.Type != nil {
			child, err := LayoutType(*variant.Type, direction)
			if err != nil {
				return ABILayout{}, fmt.Errorf("case %s: %w", variant.WITName, err)
			}
			abiCase.Layout = &child
			payloadSize = max(payloadSize, child.Size)
			payloadAlignment = max(payloadAlignment, child.Alignment)
		}
		layout.Cases = append(layout.Cases, abiCase)
	}
	layout.Alignment = max(layout.Alignment, payloadAlignment)
	layout.PayloadOffset = alignUp(4, payloadAlignment)
	layout.Size = alignUp(layout.PayloadOffset+payloadSize, layout.Alignment)
	return layout, nil
}

func resultLayout(typ model.Type, direction ABIDirection) (ABILayout, error) {
	var cases []model.Case
	if typ.Element != nil {
		cases = append(cases, model.Case{
			WITName: "ok", Type: typ.Element,
		})
	} else {
		cases = append(cases, model.Case{WITName: "ok"})
	}
	if typ.Key != nil {
		cases = append(cases, model.Case{
			WITName: "error", Type: typ.Key,
		})
	} else {
		cases = append(cases, model.Case{WITName: "error"})
	}
	layout, err := variantLayout(model.Type{Cases: cases}, direction)
	if err != nil {
		return ABILayout{}, err
	}
	layout.Kind = ABIKindResult
	return layout, nil
}

func BuildABI(api *model.API) (ABISchema, error) {
	normalized, err := normalizedAPI(api)
	if err != nil {
		return ABISchema{}, err
	}
	schema := ABISchema{
		Version: ABISchemaVersion, PointerBits: 64,
	}
	add := func(identity string, typ model.Type, direction ABIDirection) error {
		layout, err := LayoutType(typ, direction)
		if err != nil {
			return fmt.Errorf("%s: %w", identity, err)
		}
		schema.Layouts = append(schema.Layouts, ABIEntry{
			Identity: identity, Direction: direction, Layout: layout,
		})
		return nil
	}
	for _, declaration := range normalized.Declarations {
		if declaration.Coverage.State != model.CoverageRepresented {
			continue
		}
		if declaration.Type != nil {
			if err := add(declaration.Identity+".input", *declaration.Type, ABIInput); err != nil {
				return ABISchema{}, err
			}
			if err := add(declaration.Identity+".output", *declaration.Type, ABIOutput); err != nil {
				return ABISchema{}, err
			}
		}
		if declaration.Callable == nil {
			continue
		}
		for index, parameter := range declaration.Callable.Parameters {
			if err := add(
				fmt.Sprintf("%s.parameter.%d", declaration.Identity, index),
				parameter.Type,
				ABIInput,
			); err != nil {
				return ABISchema{}, err
			}
		}
		for index, result := range declaration.Callable.Results {
			if err := add(
				fmt.Sprintf("%s.result.%d", declaration.Identity, index),
				result.Type,
				ABIOutput,
			); err != nil {
				return ABISchema{}, err
			}
		}
	}
	slices.SortFunc(schema.Layouts, func(left, right ABIEntry) int {
		if left.Identity < right.Identity {
			return -1
		}
		if left.Identity > right.Identity {
			return 1
		}
		if left.Direction < right.Direction {
			return -1
		}
		if left.Direction > right.Direction {
			return 1
		}
		return 0
	})
	fingerprint, err := abiFingerprint(schema)
	if err != nil {
		return ABISchema{}, err
	}
	schema.Fingerprint = fingerprint
	return schema, nil
}

func abiFingerprint(schema ABISchema) (string, error) {
	schema.Fingerprint = ""
	encoded, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("marshal ABI schema: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func alignUp(value uint64, alignment uint32) uint64 {
	mask := uint64(alignment - 1)
	return (value + mask) &^ mask
}

func typePath(typ model.Type) string {
	if typ.Identity != "" {
		return typ.Identity
	}
	if typ.GoType != "" {
		return typ.GoType
	}
	return "<anonymous>"
}
