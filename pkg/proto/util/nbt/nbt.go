package nbt

type Nbt map[string]interface{}

func (b Nbt) Bool(name string) (ret bool, ok bool) {
	val, ok := b.Uint8(name)
	if val == 0 {
		ret = false
	} else {
		ret = true
	}
	return
}

func (b Nbt) Int8(name string) (ret int8, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(int8)
	}
	return
}
func (b Nbt) Uint8(name string) (ret uint8, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(uint8)
	}
	return
}

func (b Nbt) Int16(name string) (ret int16, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(int16)
	}
	return
}

func (b Nbt) Int32(name string) (ret int32, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(int32)
	}
	return
}

func (b Nbt) Int64(name string) (ret int64, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(int64)
	}
	return
}

func (b Nbt) Float32(name string) (ret float32, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(float32)
	}
	return
}

func (b Nbt) Float64(name string) (ret float64, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(float64)
	}
	return
}

func (b Nbt) ByteArray(name string) (ret []byte, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.([]byte)
	}
	return
}

func (b Nbt) String(name string) (ret string, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(string)
	}
	return
}

func (b Nbt) List(name string) (ret List, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(List)
	}
	return
}

func (b Nbt) Nbt(name string) (ret Nbt, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.(Nbt)
	}
	return
}

func (b Nbt) Int32Array(name string) (ret []int32, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.([]int32)
	}
	return
}

func (b Nbt) Int64Array(name string) (ret []int64, ok bool) {
	var val interface{}
	if val, ok = b[name]; ok {
		ret, ok = val.([]int64)
	}
	return
}
