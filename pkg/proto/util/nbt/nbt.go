package nbt

type Nbt map[string]interface{}
type NbtTag byte

const (
	NbtTagEnd NbtTag = iota
	NbtTagInt8
	NbtTagInt16
	NbtTagInt32
	NbtTagInt64
	NbtTagFloat32
	NbtTagFloat64
	NbtTagByteArray
	NbtTagString
	NbtTagList
	NbtTagNbt
	NbtTagInt32Array
	NbtTagInt64Array
)

func (this Nbt) Bool(name string) (ret bool, ok bool) {
	val, ok := this.Int8(name)
	if val == 0 {
		ret = false
	} else {
		ret = true
	}
	return
}

func (this Nbt) Int8(name string) (ret int8, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.(int8)
	}
	return
}

func (this Nbt) Int16(name string) (ret int16, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.(int16)
	}
	return
}

func (this Nbt) Int32(name string) (ret int32, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.(int32)
	}
	return
}

func (this Nbt) Int64(name string) (ret int64, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.(int64)
	}
	return
}

func (this Nbt) Float32(name string) (ret float32, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.(float32)
	}
	return
}

func (this Nbt) Float64(name string) (ret float64, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.(float64)
	}
	return
}

func (this Nbt) ByteArray(name string) (ret []byte, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.([]byte)
	}
	return
}

func (this Nbt) String(name string) (ret string, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.(string)
	}
	return
}

func (this Nbt) List(name string) (ret NbtList, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.(NbtList)
	}
	return
}

func (this Nbt) Nbt(name string) (ret Nbt, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.(Nbt)
	}
	return
}

func (this Nbt) Int32Array(name string) (ret []int32, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.([]int32)
	}
	return
}

func (this Nbt) Int64Array(name string) (ret []int64, ok bool) {
	var val interface{}
	if val, ok = this[name]; ok {
		ret, ok = val.([]int64)
	}
	return
}
