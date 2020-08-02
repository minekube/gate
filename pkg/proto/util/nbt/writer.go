package nbt

import (
	"errors"
	"go.minekube.com/gate/pkg/proto/util"
	"io"
)

var ErrorUnexpectedType error = errors.New("Unexpected type")

func WriteNbt(writer io.Writer, nbt Nbt) (err error) {
	return WriteNbtNamed(writer, nbt, "")
}

func WriteNbtNamed(writer io.Writer, nbt Nbt, name string) (err error) {
	if nbt == nil {
		err = util.WriteUint8(writer, byte(NbtTagEnd))
		return
	}
	err = util.WriteUint8(writer, byte(NbtTagNbt))
	if err != nil {
		return
	}
	err = writeNbtString(writer, name)
	if err != nil {
		return
	}
	err = writeNbtChild(writer, nbt, 0)
	return
}

func writeNbtString(writer io.Writer, val string) (err error) {
	err = util.WriteUint16(writer, uint16(len(val)))
	if err != nil {
		return
	}
	_, err = writer.Write([]byte(val))
	return
}

func writeNbtChild(writer io.Writer, nbt Nbt, depth int) (err error) {
	depth++
	if depth > DepthLimit {
		return ErrorLimit{"DepthLimit", depth, DepthLimit}
	}
	for name, val := range nbt {
		var tag NbtTag
		switch val.(type) {
		case bool:
			tag = NbtTagInt8
		case int8:
			tag = NbtTagInt8
		case int16:
			tag = NbtTagInt16
		case int32:
			tag = NbtTagInt32
		case int64:
			tag = NbtTagInt64
		case float32:
			tag = NbtTagFloat32
		case float64:
			tag = NbtTagFloat64
		case []byte:
			tag = NbtTagByteArray
		case string:
			tag = NbtTagString
		case NbtList:
			tag = NbtTagList
		case Nbt:
			tag = NbtTagNbt
		case []int32:
			tag = NbtTagInt32Array
		case []int64:
			tag = NbtTagInt64Array
		default:
			//panic(val)
			err = ErrorUnexpectedType
			return
		}
		err = util.WriteUint8(writer, byte(tag))
		if err != nil {
			return
		}
		err = writeNbtString(writer, name)
		if err != nil {
			return
		}
		err = writeNbtTagPayload(writer, val, depth)
		if err != nil {
			return
		}
	}
	err = util.WriteUint8(writer, byte(NbtTagEnd))
	return
}

func writeNbtTagPayload(writer io.Writer, val interface{}, depth int) (err error) {
	switch val := val.(type) {
	case bool:
		if val {
			err = util.WriteInt8(writer, 1)
		} else {
			err = util.WriteInt8(writer, 0)
		}
	case int8:
		err = util.WriteInt8(writer, val)
	case int16:
		err = util.WriteInt16(writer, val)
	case int32:
		err = util.WriteInt32(writer, val)
	case int64:
		err = util.WriteInt64(writer, val)
	case float32:
		err = util.WriteFloat32(writer, val)
	case float64:
		err = util.WriteFloat64(writer, val)
	case []byte:
		if len(val) > ByteArrayLimit {
			return ErrorLimit{"ByteArrayLimit", len(val), ByteArrayLimit}
		}
		err = util.WriteUint32(writer, uint32(len(val)))
		if err != nil {
			return
		}
		_, err = writer.Write(val)
	case string:
		err = writeNbtString(writer, val)
	case NbtList:
		depth++
		if depth > DepthLimit {
			return ErrorLimit{"DepthLimit", depth, DepthLimit}
		}
		switch list := val.array.(type) {
		case []int8:
			err = writeNbtListHeader(writer, NbtTagInt8, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case []int16:
			err = writeNbtListHeader(writer, NbtTagInt16, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case []int32:
			err = writeNbtListHeader(writer, NbtTagInt32, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case []int64:
			err = writeNbtListHeader(writer, NbtTagInt64, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case []float32:
			err = writeNbtListHeader(writer, NbtTagFloat32, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case []float64:
			err = writeNbtListHeader(writer, NbtTagFloat64, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case [][]byte:
			err = writeNbtListHeader(writer, NbtTagByteArray, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case []string:
			err = writeNbtListHeader(writer, NbtTagString, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case []NbtList:
			err = writeNbtListHeader(writer, NbtTagList, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case []Nbt:
			err = writeNbtListHeader(writer, NbtTagNbt, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case [][]int32:
			err = writeNbtListHeader(writer, NbtTagInt32Array, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		case [][]int64:
			err = writeNbtListHeader(writer, NbtTagInt64Array, len(list))
			for _, listVal := range list {
				if err != nil {
					return
				}
				err = writeNbtTagPayload(writer, listVal, depth)
			}
		}
	case Nbt:
		err = writeNbtChild(writer, val, depth)
	case []int32:
		if len(val) > Int32ArrayLimit {
			return ErrorLimit{"Int32ArrayLimit", len(val), Int32ArrayLimit}
		}
		err = util.WriteUint32(writer, uint32(len(val)))
		for _, arrayVal := range val {
			if err != nil {
				return
			}
			err = util.WriteInt32(writer, arrayVal)
		}
	case []int64:
		if len(val) > Int64ArrayLimit {
			return ErrorLimit{"Int64ArrayLimit", len(val), Int64ArrayLimit}
		}
		err = util.WriteUint32(writer, uint32(len(val)))
		for _, arrayVal := range val {
			if err != nil {
				return
			}
			err = util.WriteInt64(writer, arrayVal)
		}
	default:
		err = ErrorUnexpectedType
	}
	return
}

func writeNbtListHeader(writer io.Writer, tag NbtTag, length int) (err error) {
	err = util.WriteUint8(writer, byte(tag))
	if err != nil {
		return
	}
	if length > ListLimit {
		return ErrorLimit{"ListLimit", length, ListLimit}
	}
	err = util.WriteUint32(writer, uint32(length))
	if err != nil {
		return
	}
	return
}
