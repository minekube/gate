package config

import (
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type TagsUpdate struct {
	Tags map[string]map[string][]int
}

var _ proto.Packet = (*TagsUpdate)(nil)

func (p *TagsUpdate) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	size, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}

	p.Tags = make(map[string]map[string][]int, size)
	for i := 0; i < size; i++ {
		key, err := util.ReadString(rd)
		if err != nil {
			return err
		}

		innerSize, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}

		innerMap := make(map[string][]int, innerSize)
		for j := 0; j < innerSize; j++ {
			innerKey, err := util.ReadString(rd)
			if err != nil {
				return err
			}

			innerValue, err := util.ReadVarIntArray(rd)
			if err != nil {
				return err
			}

			innerMap[innerKey] = innerValue
		}

		p.Tags[key] = innerMap
	}

	return nil
}

func (p *TagsUpdate) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteVarInt(wr, len(p.Tags))
	if err != nil {
		return err
	}

	for key, value := range p.Tags {
		err = util.WriteString(wr, key)
		if err != nil {
			return err
		}

		err = util.WriteVarInt(wr, len(value))
		if err != nil {
			return err
		}

		for innerKey, innerValue := range value {
			err = util.WriteString(wr, innerKey)
			if err != nil {
				return err
			}

			err = util.WriteVarIntArray(wr, innerValue)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
