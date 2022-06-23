package packet

import (
	"errors"
	"fmt"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// DimensionRegistry is required for Minecraft 1.16+ clients/servers to communicate,
// it constrains the dimension types and names the client can be sent in a
// Respawn action (dimension change).
type DimensionRegistry struct {
	Dimensions []*DimensionData
	LevelNames []string
}

type DimensionInfo struct {
	RegistryIdentifier string
	LevelName          *string // nil-able
	Flat               bool
	DebugType          bool
}

const UnknownDimensionID = "gate:unknown_dimension"

type DimensionData struct {
	RegistryIdentifier         string   // the identifier for the dimension from the registry
	DimensionID                *int     // nil-able, the dimension ID contained in the registry (the "id" tag)
	Natural                    bool     // indicates if the dimension use natural world generation (e.g. overworld)
	AmbientLight               float32  // the light level the client sees without external lighting
	Shrunk                     bool     // indicates if the world is shrunk, aka not the full 256 blocks (e.g. nether)
	Ultrawarm                  bool     // internal dimension warmth flag
	Ceiling                    bool     // indicates if the dimension has a ceiling layer
	Skylight                   bool     // indicates if the dimension should display the sun
	PiglineSafe                bool     // indicates if piglins should naturally zombify in this dimension
	DoBedsWork                 bool     // indicates if players should be able to sleep in beds in this dimension
	DoRespawnAnchorsWork       bool     // indicates if player respawn points can be used in this dimension
	Raids                      bool     // indicates if raids can be spawned in the dimension
	LogicalHeight              int32    // the natural max height for the given dimension
	BurningBehaviourIdentifier string   // the identifier for how burning blocks work in the dimension
	FixedTime                  *int64   // nil-able
	CreateDragonFight          *bool    // nil-able
	CoordinateScale            *float64 // nil-able
	Effects                    *string  // optional; unknown purpose
	MinY                       *int     // Required since 1.17
	Height                     *int     // Required since 1.17
}

// fromGameData decodes a CompoundTag storing a dimension registry.
func fromGameData(toParse []util.NBT, protocol proto.Protocol) (mappings []*DimensionData, err error) {
	var data *DimensionData
	for _, compound := range toParse {
		data, err = decodeRegistryEntry(compound, protocol)
		if err != nil {
			return nil, err
		}
		mappings = append(mappings, data)
	}
	return mappings, nil
}

// Parses CompoundTag to DimensionData;
// assumes the data is part of a dimension registry.
func decodeRegistryEntry(dimTag util.NBT, protocol proto.Protocol) (*DimensionData, error) {
	registryIdentifier, ok := dimTag.String("name")
	if !ok {
		return nil, dimReadErr("data misses %q key", "name")
	}
	var (
		details     util.NBT
		dimensionID *int
	)
	if protocol.GreaterEqual(version.Minecraft_1_16_2) {
		dimID, ok := dimTag.Int("id")
		if !ok {
			return nil, dimMissKeyErr("id")
		}
		dimensionID = &dimID
		details, ok = dimTag.NBT("element")
		if !ok {
			return nil, dimMissKeyErr("element")
		}
		if details == nil {
			return nil, dimReadErr("key %q must not be nil", "element")
		}
	} else {
		details = dimTag
	}

	data, err := decodeBaseCompoundTag(details, protocol)
	if err != nil {
		return nil, err
	}
	data.RegistryIdentifier = registryIdentifier
	data.DimensionID = dimensionID
	return data, nil
}

// Parses CompoundTag to a DimensionData instance;
// assumes the data only contains dimension details.
func decodeBaseCompoundTag(details util.NBT, protocol proto.Protocol) (*DimensionData, error) {
	if details == nil {
		return nil, dimReadErr("dimension details must not be nil")
	}
	d := &DimensionData{
		RegistryIdentifier: UnknownDimensionID,
	}
	var ok bool
	d.Natural, ok = details.Bool("natural")
	if !ok {
		return nil, dimMissKeyErr("natural")
	}
	d.AmbientLight, ok = details.Float32("ambient_light")
	if !ok {
		return nil, dimMissKeyErr("ambient_light")
	}
	d.Shrunk, _ = details.Bool("shrunk")
	d.Ultrawarm, ok = details.Bool("ultrawarm")
	if !ok {
		return nil, dimMissKeyErr("ultrawarm")
	}
	d.Ceiling, ok = details.Bool("has_ceiling")
	if !ok {
		return nil, dimMissKeyErr("has_ceiling")
	}
	d.Skylight, ok = details.Bool("has_skylight")
	if !ok {
		return nil, dimMissKeyErr("has_skylight")
	}
	d.PiglineSafe, ok = details.Bool("piglin_safe")
	if !ok {
		return nil, dimMissKeyErr("piglin_safe")
	}
	d.DoBedsWork, ok = details.Bool("bed_works")
	if !ok {
		return nil, dimMissKeyErr("bed_works")
	}
	d.DoRespawnAnchorsWork, ok = details.Bool("respawn_anchor_works")
	if !ok {
		return nil, dimMissKeyErr("respawn_anchor_works")
	}
	d.Raids, ok = details.Bool("has_raids")
	if !ok {
		return nil, dimMissKeyErr("has_raids")
	}
	d.LogicalHeight, ok = details.Int32("logical_height")
	if !ok {
		return nil, dimMissKeyErr("logical_height")
	}
	d.BurningBehaviourIdentifier, ok = details.String("infiniburn")
	if !ok {
		return nil, dimMissKeyErr("infiniburn")
	}
	fixedTime, ok := details.Int64("fixed_time")
	if ok { // optional
		d.FixedTime = &fixedTime
	}
	createDragonFight, ok := details.Bool("has_enderdragon_fight")
	if ok { // optional
		d.CreateDragonFight = &createDragonFight
	}
	coordinateScale, ok := details.Float64("coordinate_scale")
	if ok {
		d.CoordinateScale = &coordinateScale
	}
	effects, ok := details.String("effects")
	if ok {
		d.Effects = &effects
	}
	minY, ok := details.Int("min_y")
	if ok {
		d.MinY = &minY
	}
	height, ok := details.Int("height")
	if ok {
		d.Height = &height
	}
	if protocol.GreaterEqual(version.Minecraft_1_17) {
		if d.MinY == nil {
			return nil, dimMissKeyErr("min_y")
		}
		if d.Height == nil {
			return nil, dimMissKeyErr("height")
		}
	}
	return d, nil
}

// utility func to create dimension decode error
func dimReadErr(format string, a ...any) error {
	return fmt.Errorf("error decoding dimension: %v", fmt.Sprintf(format, a...))
}
func dimMissKeyErr(key string) error {
	return dimReadErr("DimensionData misses %q key", key)
}

func (d *DimensionData) encodeCompoundTag(protocol proto.Protocol) (util.NBT, error) {
	details := d.encodeDimensionDetails()
	if protocol.GreaterEqual(version.Minecraft_1_16_2) {
		if d.DimensionID == nil {
			return nil, errors.New("can not encode 1.16.2+ dimension registry entry without and id")
		}
		return util.NBT{
			"name":    d.RegistryIdentifier,
			"id":      int32(*d.DimensionID),
			"element": details,
		}, nil
	}
	details["name"] = d.RegistryIdentifier
	return details, nil
}

// Encodes the Dimension data as nbt CompoundTag
func (d *DimensionData) encodeDimensionDetails() util.NBT {
	c := util.NBT{
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
	if d.CoordinateScale != nil {
		c["coordinate_scale"] = *d.CoordinateScale
	}
	if d.Effects != nil {
		c["effects"] = *d.Effects
	}
	if d.MinY != nil {
		c["min_y"] = int32(*d.MinY)
	}
	if d.Height != nil {
		c["height"] = int32(*d.Height)
	}
	return c
}

// encode the stored Dimension registry as CompoundTag containing identifier:type mappings.
func (r *DimensionRegistry) encode(protocol proto.Protocol) (dimensions []util.NBT, err error) {
	var dimensionData []util.NBT
	for i, d := range r.Dimensions {
		data, err := d.encodeCompoundTag(protocol)
		if err != nil {
			return nil, fmt.Errorf("error encoding %d. dimension: %v", i+1, err)
		}
		dimensionData = append(dimensionData, data)
	}
	return dimensionData, nil
}
