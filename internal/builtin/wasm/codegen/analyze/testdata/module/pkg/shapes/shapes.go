package shapes

import (
	"context"
	"time"
	"unsafe"
)

type NamedScalar int32

type Value struct {
	Text   string
	Values []int16
}

type Alias = Value

type Hidden struct {
	Public  string
	private int
}

type Interface interface {
	Method() string
}

type Callback func(input string) (int32, error)

type Recursive struct {
	Name string
	Next *Recursive
}

type Generic[T any] struct {
	Value T
}

type Embedded struct{}

func (Embedded) Promoted() {}

type WithEmbedded struct {
	Embedded
}

type ExampleEvent struct {
	Value string
}

func Identity[T any](value T) T {
	return value
}

func Variadic(prefix string, values ...int32) (string, error) {
	return prefix, nil
}

func Multiple() (int32, string) {
	return 0, ""
}

func UseCallback(callback Callback) error {
	_, err := callback("")
	return err
}

func (value Value) Echo(input string) string {
	return value.Text + input
}

func (value *Value) SetText(text string) {
	value.Text = text
}

const TypedConstant NamedScalar = 7

var Bool bool
var Int int
var Uintptr uintptr
var String string
var Bytes []byte
var Array [3]int16
var Map map[string]int32
var Pointer *Value
var Any any
var Channel chan<- string
var Duration time.Duration
var Timestamp time.Time
var Context context.Context
var Complex complex128
var Unsafe unsafe.Pointer
var GenericValue Generic[string]
var GenericDefinition Generic[any]
var GenericFunctionValue = Identity[int32](1)
var WritableValue Value
