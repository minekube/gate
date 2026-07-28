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

func Identity[T any](value T) T {
	return value
}

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
