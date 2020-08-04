package nbt

type List struct {
	array interface{}
}

func WrappedNbtList(array interface{}) List {
	return List{array: array}
}

func (l List) AsInt8() []int8 {
	if val, ok := l.array.([]int8); ok {
		return val
	}
	return nil
}

func (l List) AsInt16() []int16 {
	if val, ok := l.array.([]int16); ok {
		return val
	}
	return nil
}

func (l List) AsInt32() []int32 {
	if val, ok := l.array.([]int32); ok {
		return val
	}
	return nil
}

func (l List) AsInt64() []int64 {
	if val, ok := l.array.([]int64); ok {
		return val
	}
	return nil
}

func (l List) AsFloat32() []int32 {
	if val, ok := l.array.([]int32); ok {
		return val
	}
	return nil
}

func (l List) AsFloat64() []int64 {
	if val, ok := l.array.([]int64); ok {
		return val
	}
	return nil
}

func (l List) AsByteArray() [][]byte {
	if val, ok := l.array.([][]byte); ok {
		return val
	}
	return nil
}

func (l List) AsString() []string {
	if val, ok := l.array.([]string); ok {
		return val
	}
	return nil
}

func (l List) AsList() []List {
	if val, ok := l.array.([]List); ok {
		return val
	}
	return nil
}

func (l List) AsNbt() []Nbt {
	if val, ok := l.array.([]Nbt); ok {
		return val
	}
	return nil
}

func (l List) AsInt32Array() [][]int32 {
	if val, ok := l.array.([][]int32); ok {
		return val
	}
	return nil
}

func (l List) AsInt64Array() [][]int64 {
	if val, ok := l.array.([][]int64); ok {
		return val
	}
	return nil
}

func (l List) Nil() bool {
	return l.array == nil
}
