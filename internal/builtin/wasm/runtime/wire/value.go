// Package wire implements the private, deterministic value transport between
// Gate's Go host and the Rust Wasmtime adapter. The public plugin ABI remains
// the generated WIT component contract.
package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"unicode/utf8"
)

const Version byte = 1

const (
	tagNull byte = iota
	tagFalse
	tagTrue
	tagS8
	tagU8
	tagS16
	tagU16
	tagS32
	tagU32
	tagS64
	tagU64
	tagF32
	tagF64
	tagChar
	tagString
	tagList
	tagMap
	tagRecord
	tagTuple
	tagVariant
	tagEnum
	tagFlags
	tagResource
	tagResult
)

const maxCollectionElements = 1 << 24

type Field struct {
	Name  string
	Value any
}

type Record []Field

type Pair struct {
	Key   any
	Value any
}

type Map []Pair
type Tuple []any

type Variant struct {
	Name     string
	Value    any
	HasValue bool
}

type Enum string
type Flags []string
type Resource uint64
type Char rune

type Result struct {
	Value    any
	IsError  bool
	HasValue bool
}

type GateError struct {
	Kind      string
	Message   string
	Operation string
}

type Response struct {
	Values []any
	Error  *GateError
}

func Encode(values []any) ([]byte, error) {
	var output bytes.Buffer
	output.WriteByte(Version)
	writeUint(&output, uint64(len(values)))
	for _, value := range values {
		if err := encodeValue(&output, value); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func Decode(input []byte) ([]any, error) {
	reader := bytes.NewReader(input)
	if err := readVersion(reader); err != nil {
		return nil, err
	}
	count, err := readCount(reader, "value")
	if err != nil {
		return nil, err
	}
	values := make([]any, count)
	for index := range values {
		values[index], err = decodeValue(reader)
		if err != nil {
			return nil, fmt.Errorf("value %d: %w", index, err)
		}
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("wire input has %d trailing bytes", reader.Len())
	}
	return values, nil
}

func EncodeResponse(response Response) ([]byte, error) {
	var output bytes.Buffer
	output.WriteByte(Version)
	if response.Error != nil {
		output.WriteByte(1)
		if err := writeString(&output, response.Error.Kind); err != nil {
			return nil, err
		}
		if err := writeString(&output, response.Error.Message); err != nil {
			return nil, err
		}
		if err := writeString(&output, response.Error.Operation); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}
	output.WriteByte(0)
	writeUint(&output, uint64(len(response.Values)))
	for _, value := range response.Values {
		if err := encodeValue(&output, value); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func DecodeResponse(input []byte) (Response, error) {
	reader := bytes.NewReader(input)
	if err := readVersion(reader); err != nil {
		return Response{}, err
	}
	status, err := reader.ReadByte()
	if err != nil {
		return Response{}, errors.New("wire response status is missing")
	}
	var response Response
	switch status {
	case 0:
		count, err := readCount(reader, "response value")
		if err != nil {
			return Response{}, err
		}
		response.Values = make([]any, count)
		for index := range response.Values {
			response.Values[index], err = decodeValue(reader)
			if err != nil {
				return Response{}, fmt.Errorf("response value %d: %w", index, err)
			}
		}
	case 1:
		kind, err := readString(reader)
		if err != nil {
			return Response{}, fmt.Errorf("response error kind: %w", err)
		}
		message, err := readString(reader)
		if err != nil {
			return Response{}, fmt.Errorf("response error message: %w", err)
		}
		operation, err := readString(reader)
		if err != nil {
			return Response{}, fmt.Errorf("response error operation: %w", err)
		}
		response.Error = &GateError{
			Kind: kind, Message: message, Operation: operation,
		}
	default:
		return Response{}, fmt.Errorf("unknown wire response status %d", status)
	}
	if reader.Len() != 0 {
		return Response{}, fmt.Errorf("wire response has %d trailing bytes", reader.Len())
	}
	return response, nil
}

func encodeValue(output *bytes.Buffer, value any) error {
	switch value := value.(type) {
	case nil:
		output.WriteByte(tagNull)
	case bool:
		if value {
			output.WriteByte(tagTrue)
		} else {
			output.WriteByte(tagFalse)
		}
	case int8:
		output.WriteByte(tagS8)
		output.WriteByte(byte(value))
	case uint8:
		output.WriteByte(tagU8)
		output.WriteByte(value)
	case int16:
		output.WriteByte(tagS16)
		writeFixed(output, uint16(value))
	case uint16:
		output.WriteByte(tagU16)
		writeFixed(output, value)
	case int32:
		output.WriteByte(tagS32)
		writeFixed(output, uint32(value))
	case uint32:
		output.WriteByte(tagU32)
		writeFixed(output, value)
	case int, int64:
		output.WriteByte(tagS64)
		if integer, ok := value.(int); ok {
			writeFixed(output, uint64(int64(integer)))
		} else {
			writeFixed(output, uint64(value.(int64)))
		}
	case uint, uint64:
		output.WriteByte(tagU64)
		if integer, ok := value.(uint); ok {
			writeFixed(output, uint64(integer))
		} else {
			writeFixed(output, value.(uint64))
		}
	case float32:
		output.WriteByte(tagF32)
		writeFixed(output, math.Float32bits(value))
	case float64:
		output.WriteByte(tagF64)
		writeFixed(output, math.Float64bits(value))
	case Char:
		output.WriteByte(tagChar)
		writeFixed(output, uint32(value))
	case string:
		output.WriteByte(tagString)
		return writeString(output, value)
	case []any:
		output.WriteByte(tagList)
		return encodeSequence(output, value)
	case Map:
		output.WriteByte(tagMap)
		writeUint(output, uint64(len(value)))
		for _, pair := range value {
			if err := encodeValue(output, pair.Key); err != nil {
				return err
			}
			if err := encodeValue(output, pair.Value); err != nil {
				return err
			}
		}
	case Record:
		output.WriteByte(tagRecord)
		writeUint(output, uint64(len(value)))
		for _, field := range value {
			if err := writeString(output, field.Name); err != nil {
				return err
			}
			if err := encodeValue(output, field.Value); err != nil {
				return err
			}
		}
	case Tuple:
		output.WriteByte(tagTuple)
		return encodeSequence(output, []any(value))
	case Variant:
		output.WriteByte(tagVariant)
		if err := writeString(output, value.Name); err != nil {
			return err
		}
		if value.HasValue {
			output.WriteByte(1)
			return encodeValue(output, value.Value)
		}
		output.WriteByte(0)
	case Enum:
		output.WriteByte(tagEnum)
		return writeString(output, string(value))
	case Flags:
		output.WriteByte(tagFlags)
		writeUint(output, uint64(len(value)))
		for _, flag := range value {
			if err := writeString(output, flag); err != nil {
				return err
			}
		}
	case Resource:
		output.WriteByte(tagResource)
		writeFixed(output, uint64(value))
	case Result:
		output.WriteByte(tagResult)
		var discriminant byte
		if value.IsError {
			discriminant |= 1
		}
		if value.HasValue {
			discriminant |= 2
		}
		output.WriteByte(discriminant)
		if value.HasValue {
			return encodeValue(output, value.Value)
		}
	default:
		return fmt.Errorf("unsupported wire value %T", value)
	}
	return nil
}

func encodeSequence(output *bytes.Buffer, values []any) error {
	writeUint(output, uint64(len(values)))
	for _, value := range values {
		if err := encodeValue(output, value); err != nil {
			return err
		}
	}
	return nil
}

func decodeValue(reader *bytes.Reader) (any, error) {
	tag, err := reader.ReadByte()
	if err != nil {
		return nil, io.ErrUnexpectedEOF
	}
	switch tag {
	case tagNull:
		return nil, nil
	case tagFalse:
		return false, nil
	case tagTrue:
		return true, nil
	case tagS8:
		value, err := reader.ReadByte()
		return int8(value), err
	case tagU8:
		return reader.ReadByte()
	case tagS16:
		value, err := readFixed[uint16](reader)
		return int16(value), err
	case tagU16:
		return readFixed[uint16](reader)
	case tagS32:
		value, err := readFixed[uint32](reader)
		return int32(value), err
	case tagU32:
		return readFixed[uint32](reader)
	case tagS64:
		value, err := readFixed[uint64](reader)
		return int64(value), err
	case tagU64:
		return readFixed[uint64](reader)
	case tagF32:
		value, err := readFixed[uint32](reader)
		return math.Float32frombits(value), err
	case tagF64:
		value, err := readFixed[uint64](reader)
		return math.Float64frombits(value), err
	case tagChar:
		value, err := readFixed[uint32](reader)
		if err != nil {
			return nil, err
		}
		character := rune(value)
		if !utf8.ValidRune(character) {
			return nil, fmt.Errorf("invalid character U+%X", value)
		}
		return Char(character), nil
	case tagString:
		return readString(reader)
	case tagList:
		return decodeSequence(reader)
	case tagMap:
		count, err := readCount(reader, "map")
		if err != nil {
			return nil, err
		}
		value := make(Map, count)
		for index := range value {
			value[index].Key, err = decodeValue(reader)
			if err != nil {
				return nil, fmt.Errorf("map key %d: %w", index, err)
			}
			value[index].Value, err = decodeValue(reader)
			if err != nil {
				return nil, fmt.Errorf("map value %d: %w", index, err)
			}
		}
		return value, nil
	case tagRecord:
		count, err := readCount(reader, "record")
		if err != nil {
			return nil, err
		}
		value := make(Record, count)
		for index := range value {
			value[index].Name, err = readString(reader)
			if err != nil {
				return nil, fmt.Errorf("record field %d name: %w", index, err)
			}
			value[index].Value, err = decodeValue(reader)
			if err != nil {
				return nil, fmt.Errorf("record field %q: %w", value[index].Name, err)
			}
		}
		return value, nil
	case tagTuple:
		value, err := decodeSequence(reader)
		return Tuple(value), err
	case tagVariant:
		name, err := readString(reader)
		if err != nil {
			return nil, err
		}
		hasValue, err := reader.ReadByte()
		if err != nil {
			return nil, io.ErrUnexpectedEOF
		}
		variant := Variant{Name: name}
		switch hasValue {
		case 0:
			return variant, nil
		case 1:
			variant.HasValue = true
			variant.Value, err = decodeValue(reader)
			return variant, err
		default:
			return nil, fmt.Errorf("invalid variant payload flag %d", hasValue)
		}
	case tagEnum:
		value, err := readString(reader)
		return Enum(value), err
	case tagFlags:
		count, err := readCount(reader, "flags")
		if err != nil {
			return nil, err
		}
		value := make(Flags, count)
		for index := range value {
			value[index], err = readString(reader)
			if err != nil {
				return nil, fmt.Errorf("flag %d: %w", index, err)
			}
		}
		return value, nil
	case tagResource:
		value, err := readFixed[uint64](reader)
		return Resource(value), err
	case tagResult:
		discriminant, err := reader.ReadByte()
		if err != nil {
			return nil, io.ErrUnexpectedEOF
		}
		if discriminant&^byte(3) != 0 {
			return nil, fmt.Errorf("invalid result discriminant %d", discriminant)
		}
		value := Result{
			IsError:  discriminant&1 != 0,
			HasValue: discriminant&2 != 0,
		}
		if value.HasValue {
			value.Value, err = decodeValue(reader)
		}
		return value, err
	default:
		return nil, fmt.Errorf("unknown wire value tag %d", tag)
	}
}

func decodeSequence(reader *bytes.Reader) ([]any, error) {
	count, err := readCount(reader, "list")
	if err != nil {
		return nil, err
	}
	values := make([]any, count)
	for index := range values {
		values[index], err = decodeValue(reader)
		if err != nil {
			return nil, fmt.Errorf("list item %d: %w", index, err)
		}
	}
	return values, nil
}

func readVersion(reader *bytes.Reader) error {
	version, err := reader.ReadByte()
	if err != nil {
		return errors.New("wire version is missing")
	}
	if version != Version {
		return fmt.Errorf("unsupported wire version %d", version)
	}
	return nil
}

func writeString(output *bytes.Buffer, value string) error {
	if !utf8.ValidString(value) {
		return errors.New("wire string is not valid UTF-8")
	}
	writeUint(output, uint64(len(value)))
	output.WriteString(value)
	return nil
}

func readString(reader *bytes.Reader) (string, error) {
	length, err := readLength(reader, "string")
	if err != nil {
		return "", err
	}
	if length > reader.Len() {
		return "", fmt.Errorf("string length %d exceeds %d remaining bytes", length, reader.Len())
	}
	value := make([]byte, length)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	if !utf8.Valid(value) {
		return "", errors.New("wire string is not valid UTF-8")
	}
	return string(value), nil
}

func writeUint(output *bytes.Buffer, value uint64) {
	var encoded [binary.MaxVarintLen64]byte
	length := binary.PutUvarint(encoded[:], value)
	output.Write(encoded[:length])
}

func readCount(reader *bytes.Reader, kind string) (int, error) {
	count, err := readLength(reader, kind)
	if err != nil {
		return 0, err
	}
	if count > maxCollectionElements {
		return 0, fmt.Errorf("%s has too many elements: %d", kind, count)
	}
	return count, nil
}

func readLength(reader *bytes.Reader, kind string) (int, error) {
	value, err := binary.ReadUvarint(reader)
	if err != nil {
		return 0, fmt.Errorf("%s length: %w", kind, err)
	}
	if value > uint64(^uint(0)>>1) {
		return 0, fmt.Errorf("%s length does not fit int", kind)
	}
	return int(value), nil
}

func writeFixed[T uint16 | uint32 | uint64](output *bytes.Buffer, value T) {
	var encoded [8]byte
	switch value := any(value).(type) {
	case uint16:
		binary.LittleEndian.PutUint16(encoded[:], value)
		output.Write(encoded[:2])
	case uint32:
		binary.LittleEndian.PutUint32(encoded[:], value)
		output.Write(encoded[:4])
	case uint64:
		binary.LittleEndian.PutUint64(encoded[:], value)
		output.Write(encoded[:8])
	}
}

func readFixed[T uint16 | uint32 | uint64](reader *bytes.Reader) (T, error) {
	var zero T
	size := int(binary.Size(zero))
	var encoded [8]byte
	if _, err := io.ReadFull(reader, encoded[:size]); err != nil {
		return zero, io.ErrUnexpectedEOF
	}
	switch any(zero).(type) {
	case uint16:
		return T(binary.LittleEndian.Uint16(encoded[:])), nil
	case uint32:
		return T(binary.LittleEndian.Uint32(encoded[:])), nil
	case uint64:
		return T(binary.LittleEndian.Uint64(encoded[:])), nil
	default:
		panic("unreachable fixed-width type")
	}
}
