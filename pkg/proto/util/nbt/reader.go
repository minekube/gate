package nbt

import (
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/proto/util"
	"io"
)

type ErrorUnexpectedNbtTag NbtTag

func (this ErrorUnexpectedNbtTag) Error() string {
	return fmt.Sprintf("Unexpected nbt tag: %d", this)
}

func ReadNbt(reader io.Reader) (nbt Nbt, err error) {
	nbt, _, err = ReadNbtNamed(reader)
	return
}

func ReadNbtNamed(reader io.Reader) (nbt Nbt, name string, err error) {
	tagId, err := util.ReadUint8(reader)
	if err != nil {
		return
	}
	tag := NbtTag(tagId)
	if tag == NbtTagNbt {
		name, err = readNbtString(reader)
		if err != nil {
			return
		}
		nbt, err = readNbtChild(reader, -1)
	} else if tag == NbtTagEnd {
		nbt = nil
	} else {
		err = ErrorUnexpectedNbtTag(tag)
	}
	return
}

func readNbtString(reader io.Reader) (val string, err error) {
	length, err := util.ReadUint16(reader)
	if err != nil {
		return
	}
	bytes := make([]byte, length)
	_, err = reader.Read(bytes)
	if err != nil {
		return
	}
	val = string(bytes)
	return
}

func readNbtChild(reader io.Reader, depth int) (nbt Nbt, err error) {
	depth++
	if depth > DepthLimit {
		err = ErrorLimit{"DepthLimit", depth, DepthLimit}
		return
	}
	nbt = make(Nbt)
	var tagId byte
	var name string
	for {
		tagId, err = util.ReadUint8(reader)
		if err != nil {
			return
		}
		if tagId == byte(NbtTagEnd) {
			return
		}
		name, err = readNbtString(reader)
		if err != nil {
			return
		}
		nbt[name], err = readNbtTagPayload(reader, NbtTag(tagId), depth)
		if err != nil {
			return
		}
	}
}

func readNbtTagPayload(reader io.Reader, tag NbtTag, depth int) (val interface{}, err error) {
	switch tag {
	case NbtTagInt8:
		val, err = util.ReadInt8(reader)
	case NbtTagInt16:
		val, err = util.ReadInt16(reader)
	case NbtTagInt32:
		val, err = util.ReadInt32(reader)
	case NbtTagInt64:
		val, err = util.ReadInt64(reader)
	case NbtTagFloat32:
		val, err = util.ReadFloat32(reader)
	case NbtTagFloat64:
		val, err = util.ReadFloat64(reader)
	case NbtTagByteArray:
		var arrayLength uint32
		arrayLength, err = util.ReadUint32(reader)
		if err != nil {
			return
		}
		if int(arrayLength) > ByteArrayLimit {
			err = ErrorLimit{"ByteArrayLimit", int(arrayLength), ByteArrayLimit}
			return
		}
		bytes := make([]byte, arrayLength)
		_, err = reader.Read(bytes)
		val = bytes
	case NbtTagString:
		val, err = readNbtString(reader)
	case NbtTagList:
		depth++
		if depth > DepthLimit {
			err = ErrorLimit{"DepthLimit", depth, DepthLimit}
			return
		}
		var listTagId byte
		listTagId, err = util.ReadUint8(reader)
		if err != nil {
			return
		}
		listTag := NbtTag(listTagId)
		var listLength uint32
		listLength, err = util.ReadUint32(reader)
		if err != nil {
			return
		}
		if int(listLength) > ListLimit {
			err = ErrorLimit{"ListLimit", int(listLength), ListLimit}
			return
		}
		// TODO Holy hell of programming this is ridiculously ugly
		//    - try to fix this?
		switch listTag {
		case NbtTagEnd:
			val = WrappedNbtList(make([]Nbt, 0))
		case NbtTagInt8:
			list := make([]int8, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.(int8)
			}
			val = WrappedNbtList(list)
		case NbtTagInt16:
			list := make([]int16, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.(int16)
			}
			val = WrappedNbtList(list)
		case NbtTagInt32:
			list := make([]int32, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.(int32)
			}
			val = WrappedNbtList(list)
		case NbtTagInt64:
			list := make([]int64, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.(int64)
			}
			val = WrappedNbtList(list)
		case NbtTagFloat32:
			list := make([]float32, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.(float32)
			}
			val = WrappedNbtList(list)
		case NbtTagFloat64:
			list := make([]float64, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.(float64)
			}
			val = WrappedNbtList(list)
		case NbtTagByteArray:
			list := make([][]byte, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.([]byte)
			}
			val = WrappedNbtList(list)
		case NbtTagString:
			list := make([]string, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.(string)
			}
			val = WrappedNbtList(list)
		case NbtTagList:
			list := make([]NbtList, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.(NbtList)
			}
			val = WrappedNbtList(list)
		case NbtTagNbt:
			list := make([]Nbt, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.(Nbt)
			}
			val = WrappedNbtList(list)
		case NbtTagInt32Array:
			list := make([][]int32, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.([]int32)
			}
			val = WrappedNbtList(list)
		case NbtTagInt64Array:
			list := make([][]int64, listLength)
			var listVal interface{}
			for i := range list {
				listVal, err = readNbtTagPayload(reader, listTag, depth)
				if err != nil {
					return
				}
				list[i] = listVal.([]int64)
			}
			val = WrappedNbtList(list)
		default:
			err = ErrorUnexpectedNbtTag(listTag)
		}
	case NbtTagNbt:
		val, err = readNbtChild(reader, depth)
	case NbtTagInt32Array:
		var arrayLength uint32
		arrayLength, err = util.ReadUint32(reader)
		if err != nil {
			return
		}
		if int(arrayLength) > Int32ArrayLimit {
			err = ErrorLimit{"Int32ArrayLimit", int(arrayLength), Int32ArrayLimit}
			return
		}
		array := make([]int32, arrayLength)
		for i := range array {
			array[i], err = util.ReadInt32(reader)
			if err != nil {
				return
			}
		}
		val = array
	case NbtTagInt64Array:
		var arrayLength uint32
		arrayLength, err = util.ReadUint32(reader)
		if err != nil {
			return
		}
		if int(arrayLength) > Int64ArrayLimit {
			err = ErrorLimit{"Int64ArrayLimit", int(arrayLength), Int64ArrayLimit}
			return
		}
		array := make([]int64, arrayLength)
		for i := range array {
			array[i], err = util.ReadInt64(reader)
			if err != nil {
				return
			}
		}
		val = array
	default:
		err = ErrorUnexpectedNbtTag(tag)
	}
	return
}

func ReadCompoundTag(rd io.Reader) (Nbt, error) {
	nbt, err := ReadNbt(rd)
	if err != nil {
		return nil, err
	}
	if nbt == nil {
		return nil, errors.New("invalid NBT start-type (end/empty)")
	}
	return ReadNbt(rd)
}
