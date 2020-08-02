package nbt

type NbtList struct {
	array interface{}
}

func WrappedNbtList(array interface{}) NbtList {
	return NbtList{array: array}
}

func (this NbtList) AsInt8() []int8 {
	if val, ok := this.array.([]int8); ok {
		return val
	}
	return nil
}

func (this NbtList) AsInt16() []int16 {
	if val, ok := this.array.([]int16); ok {
		return val
	}
	return nil
}

func (this NbtList) AsInt32() []int32 {
	if val, ok := this.array.([]int32); ok {
		return val
	}
	return nil
}

func (this NbtList) AsInt64() []int64 {
	if val, ok := this.array.([]int64); ok {
		return val
	}
	return nil
}

func (this NbtList) AsFloat32() []int32 {
	if val, ok := this.array.([]int32); ok {
		return val
	}
	return nil
}

func (this NbtList) AsFloat64() []int64 {
	if val, ok := this.array.([]int64); ok {
		return val
	}
	return nil
}

func (this NbtList) AsByteArray() [][]byte {
	if val, ok := this.array.([][]byte); ok {
		return val
	}
	return nil
}

func (this NbtList) AsString() []string {
	if val, ok := this.array.([]string); ok {
		return val
	}
	return nil
}

func (this NbtList) AsList() []NbtList {
	if val, ok := this.array.([]NbtList); ok {
		return val
	}
	return nil
}

func (this NbtList) AsNbt() []Nbt {
	if val, ok := this.array.([]Nbt); ok {
		return val
	}
	return nil
}

func (this NbtList) AsInt32Array() [][]int32 {
	if val, ok := this.array.([][]int32); ok {
		return val
	}
	return nil
}

func (this NbtList) AsInt64Array() [][]int64 {
	if val, ok := this.array.([][]int64); ok {
		return val
	}
	return nil
}

func (this NbtList) Nil() bool {
	return this.array == nil
}
