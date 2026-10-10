package brigadier

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// minecraft263ArgumentTypes is the vanilla 26.3 minecraft:command_argument_type
// registry in protocol id order, as reported by the 26.3 server data generator
// (java -DbundlerMainClass=net.minecraft.data.Main -jar server.jar --reports).
// 26.3 inserted context_float_provider, context_int_provider, slot_source,
// feature and swing_animation, which moved dialog and uuid.
var minecraft263ArgumentTypes = []string{
	"brigadier:bool",
	"brigadier:float",
	"brigadier:double",
	"brigadier:integer",
	"brigadier:long",
	"brigadier:string",
	"minecraft:entity",
	"minecraft:game_profile",
	"minecraft:block_pos",
	"minecraft:column_pos",
	"minecraft:vec3",
	"minecraft:vec2",
	"minecraft:block_state",
	"minecraft:block_predicate",
	"minecraft:item_stack",
	"minecraft:item_predicate",
	"minecraft:team_color",
	"minecraft:hex_color",
	"minecraft:component",
	"minecraft:style",
	"minecraft:message",
	"minecraft:nbt_compound_tag",
	"minecraft:nbt_tag",
	"minecraft:nbt_path",
	"minecraft:objective",
	"minecraft:objective_criteria",
	"minecraft:operation",
	"minecraft:particle",
	"minecraft:angle",
	"minecraft:rotation",
	"minecraft:scoreboard_slot",
	"minecraft:score_holder",
	"minecraft:swizzle",
	"minecraft:team",
	"minecraft:item_slot",
	"minecraft:item_slots",
	"minecraft:resource_location",
	"minecraft:function",
	"minecraft:entity_anchor",
	"minecraft:int_range",
	"minecraft:float_range",
	"minecraft:dimension",
	"minecraft:gamemode",
	"minecraft:time",
	"minecraft:resource_or_tag",
	"minecraft:resource_or_tag_key",
	"minecraft:resource",
	"minecraft:resource_key",
	"minecraft:resource_selector",
	"minecraft:template_mirror",
	"minecraft:template_rotation",
	"minecraft:heightmap",
	"minecraft:loot_table",
	"minecraft:loot_predicate",
	"minecraft:loot_modifier",
	"minecraft:context_float_provider",
	"minecraft:context_int_provider",
	"minecraft:slot_source",
	"minecraft:dialog",
	"minecraft:feature",
	"minecraft:swing_animation",
	"minecraft:uuid",
}

func TestMinecraft263ArgumentTypeIDs(t *testing.T) {
	// 62 entries, ids 0-61. Without this a removed tail entry would just
	// drop its subtest and the table would still pass.
	require.Len(t, minecraft263ArgumentTypes, 62)

	protocol := version.Minecraft_26_3.Protocol
	for id, name := range minecraft263ArgumentTypes {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, util.WriteVarInt(&buf, id))

			identifier, err := registry.readIdentifier(&buf, protocol)
			require.NoError(t, err)
			require.Equal(t, name, identifier.id)

			var out bytes.Buffer
			require.NoError(t, registry.writeIdentifier(&out, identifier, protocol))
			got, err := util.ReadVarInt(&out)
			require.NoError(t, err)
			require.Equal(t, id, got)
		})
	}
}

func TestDialogAndUUIDKeepTheirIDsBeforeMinecraft263(t *testing.T) {
	for _, protocol := range []proto.Protocol{version.Minecraft_1_21_6.Protocol, version.Minecraft_26_2.Protocol} {
		for _, tc := range []struct {
			id   int
			name string
		}{
			{55, "minecraft:dialog"},
			{56, "minecraft:uuid"},
		} {
			var buf bytes.Buffer
			require.NoError(t, util.WriteVarInt(&buf, tc.id))

			identifier, err := registry.readIdentifier(&buf, protocol)
			require.NoError(t, err)
			require.Equal(t, tc.name, identifier.id, "protocol %d id %d", protocol, tc.id)
		}
	}
}
