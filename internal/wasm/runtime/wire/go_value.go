package wire

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"go.minekube.com/gate/internal/wasm/runtime/resources"
)

// MarshalGoValues copies ordinary Go values into wire values. Pointer,
// interface, function, and channel identities are kept in the plugin's
// resource table and represented only by generation-checked handles.
func MarshalGoValues(values []any, table *resources.Table) ([]any, error) {
	marshaled := make([]any, len(values))
	for index, value := range values {
		var err error
		marshaled[index], err = marshalGoValue(reflect.ValueOf(value), table)
		if err != nil {
			return nil, fmt.Errorf("result %d: %w", index, err)
		}
	}
	return marshaled, nil
}

func marshalGoValue(value reflect.Value, table *resources.Table) (any, error) {
	if !value.IsValid() {
		return nil, nil
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil, nil
		}
		return insertResource(value.Interface(), value.Type(), table)
	}
	switch value.Kind() {
	case reflect.Bool:
		return value.Bool(), nil
	case reflect.Int8:
		return int8(value.Int()), nil
	case reflect.Int16:
		return int16(value.Int()), nil
	case reflect.Int32:
		return int32(value.Int()), nil
	case reflect.Int, reflect.Int64:
		return value.Int(), nil
	case reflect.Uint8:
		return uint8(value.Uint()), nil
	case reflect.Uint16:
		return uint16(value.Uint()), nil
	case reflect.Uint32:
		return uint32(value.Uint()), nil
	case reflect.Uint, reflect.Uint64, reflect.Uintptr:
		return value.Uint(), nil
	case reflect.Float32:
		return float32(value.Float()), nil
	case reflect.Float64:
		return value.Float(), nil
	case reflect.Complex64, reflect.Complex128:
		number := value.Complex()
		return Record{
			{Name: "real", Value: real(number)},
			{Name: "imaginary", Value: imag(number)},
		}, nil
	case reflect.String:
		return value.String(), nil
	case reflect.Pointer, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		if value.IsNil() {
			return nil, nil
		}
		return insertResource(value.Interface(), value.Type(), table)
	case reflect.Slice:
		if value.IsNil() {
			return nil, nil
		}
		return marshalSequence(value, table)
	case reflect.Array:
		return marshalSequence(value, table)
	case reflect.Map:
		if value.IsNil() {
			return nil, nil
		}
		entries := make(Map, 0, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			key, err := marshalGoValue(iterator.Key(), table)
			if err != nil {
				return nil, fmt.Errorf("map key: %w", err)
			}
			item, err := marshalGoValue(iterator.Value(), table)
			if err != nil {
				return nil, fmt.Errorf("map value: %w", err)
			}
			entries = append(entries, Pair{Key: key, Value: item})
		}
		return entries, nil
	case reflect.Struct:
		record := make(Record, 0, value.NumField())
		typ := value.Type()
		for index := 0; index < value.NumField(); index++ {
			field := typ.Field(index)
			if !field.IsExported() {
				continue
			}
			item, err := marshalGoValue(value.Field(index), table)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, err)
			}
			record = append(record, Field{
				Name: canonicalName(field.Name), Value: item,
			})
		}
		return record, nil
	default:
		return nil, fmt.Errorf("unsupported Go value %s", value.Type())
	}
}

func marshalSequence(value reflect.Value, table *resources.Table) ([]any, error) {
	items := make([]any, value.Len())
	for index := range items {
		item, err := marshalGoValue(value.Index(index), table)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", index, err)
		}
		items[index] = item
	}
	return items, nil
}

func insertResource(
	value any,
	typ reflect.Type,
	table *resources.Table,
) (Resource, error) {
	if table == nil {
		return 0, fmt.Errorf("cannot marshal resource %s without a resource table", typ)
	}
	identity := resourceTypeIdentity(typ)
	handle, err := table.Insert(
		value,
		identity,
		resources.LifetimeOwned,
		nil,
	)
	return Resource(handle), err
}

func resourceTypeIdentity(typ reflect.Type) string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.PkgPath() == "" {
		return typ.String()
	}
	return typ.PkgPath() + "." + typ.Name()
}

func canonicalName(input string) string {
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
		if current > unicode.MaxASCII || !isASCIIAlphaNumeric(current) {
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
	return strings.Join(words, "-")
}

func isASCIIAlphaNumeric(value rune) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
}
