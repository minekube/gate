package packet

import (
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/proto/util"
	"go.minekube.com/gate/pkg/util/sets"
)

// DimensionRegistry is required for Minecraft 1.16+ clients/servers to communicate,
// it constrains the dimension types and names the client can be sent in a
// Respawn action (dimension change).
type DimensionRegistry struct {
	Dimensions []*DimensionData
	LevelNames sets.String
}

type DimensionInfo struct {
	RegistryId string
	LevelName  string
	Flat       bool
	DebugType  bool
}

type DimensionData struct {
	RegistryIdentifier string
	AmbientLight       float32
	Shrunk, Natural, Ultrawarm, Ceiling, Skylight, PiglineSafe,
	DoBedsWork, DoRespawnAnchorsWork, Raids bool
	LogicalHeight              int32
	BurningBehaviourIdentifier string
	FixedTime                  *int64 // nil-able
	CreateDragonFight          *bool  // nil-able
}

// FromGameData decodes a CompoundTag storing a dimension registry.
func FromGameData(toParse util.NBT) (mappings []*DimensionData, err error) {
	if toParse == nil {
		return nil, errors.New("gamedata is cannot be nil")
	}
	dimension, ok := toParse["dimension"]
	if !ok {
		return nil, errors.New("gamedata does not contain dimension")
	}
	list, ok := dimension.([]interface{})
	if !ok {
		return nil, errors.New("gamedata dimension is not a list")
	}
	var data *DimensionData
	for i, compound := range list {
		compound, ok := compound.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("dimension data at index %d is not a compount nbt", i)
		}
		data, err = DecodeCompoundTagDimensionData(compound)
		if err != nil {
			return nil, err
		}
		mappings = append(mappings, data)
	}
	return mappings, nil
}

func DecodeCompoundTagDimensionData(toRead util.NBT) (*DimensionData, error) {
	if toRead == nil {
		return nil, errors.New("CompoundTag cannot be nil")
	}
	err := func(key string) error { return fmt.Errorf("CompoundTag is missing DimensionData %q", key) }
	d := &DimensionData{}
	var ok bool
	d.RegistryIdentifier, ok = toRead.String("name")
	if !ok {
		return nil, err("name")
	}
	d.Natural, ok = toRead.Bool("natural")
	if !ok {
		return nil, err("natural")
	}
	d.AmbientLight, ok = toRead.Float32("ambient_light")
	if !ok {
		return nil, err("ambient_light")
	}
	d.Shrunk, ok = toRead.Bool("shrunk")
	if !ok {
		return nil, err("shrunk")
	}
	d.Ultrawarm, ok = toRead.Bool("ultrawarm")
	if !ok {
		return nil, err("ultrawarm")
	}
	d.Ceiling, ok = toRead.Bool("has_ceiling")
	if !ok {
		return nil, err("has_ceiling")
	}
	d.Skylight, ok = toRead.Bool("has_skylight")
	if !ok {
		return nil, err("has_skylight")
	}
	d.PiglineSafe, ok = toRead.Bool("piglin_safe")
	if !ok {
		return nil, err("piglin_safe")
	}
	d.DoBedsWork, ok = toRead.Bool("bed_works")
	if !ok {
		return nil, err("bed_works")
	}
	d.DoRespawnAnchorsWork, ok = toRead.Bool("respawn_anchor_works")
	if !ok {
		return nil, err("respawn_anchor_works")
	}
	d.Raids, ok = toRead.Bool("has_raids")
	if !ok {
		return nil, err("has_raids")
	}
	d.LogicalHeight, ok = toRead.Int32("logical_height")
	if !ok {
		return nil, err("logical_height")
	}
	d.BurningBehaviourIdentifier, ok = toRead.String("infiniburn")
	if !ok {
		return nil, err("infiniburn")
	}
	fixedTime, ok := toRead.Int64("fixed_time")
	if ok { // optional
		d.FixedTime = &fixedTime
	}
	createDragonFight, ok := toRead.Bool("has_enderdragon_fight")
	if ok { // optional
		d.CreateDragonFight = &createDragonFight
	}
	return d, nil
}

// Encodes the Dimension data as nbt CompoundTag
func (d *DimensionData) EncodeCompoundTag() util.NBT {
	c := util.NBT{
		"name":                 d.RegistryIdentifier,
		"natural":              d.Natural,
		"ambient_light":        d.AmbientLight,
		"shrunk":               d.Shrunk,
		"ultrawarm":            d.Ultrawarm,
		"has_ceiling":          d.Ceiling,
		"has_skylight":         d.Skylight,
		"piglin_safe":          d.PiglineSafe,
		"bed_works":            d.DoBedsWork,
		"respawn_anchor_works": d.DoRespawnAnchorsWork,
		"has_raids":            d.Raids,
		"logical_height":       d.LogicalHeight,
		"infiniburn":           d.BurningBehaviourIdentifier,
	}
	if d.FixedTime != nil {
		c["fixed_time"] = *d.FixedTime
	}
	if d.CreateDragonFight != nil {
		c["has_enderdragon_fight"] = *d.CreateDragonFight
	}
	return c
}

// ToNBT the stored Dimension registry as CompoundTag containing identifier:type mappings.
func (r *DimensionRegistry) ToNBT() util.NBT {
	var dimensionData []util.NBT
	for _, d := range r.Dimensions {
		dimensionData = append(dimensionData, d.EncodeCompoundTag())
	}
	return util.NBT{"dimension": dimensionData}
}
