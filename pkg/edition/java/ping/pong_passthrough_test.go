package ping

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
)

// forge1201StatusJSON is the server list response shape of a Forge 1.20.1
// (FML3) backend. Forge keeps "channels" and "mods" in the block for
// backwards compatibility but leaves them empty since 1.20 and encodes the
// real mod/channel inventory into the compressed "d" member
// (net.minecraftforge.network.ServerStatusPing#encodeOptimized).
const forge1201StatusJSON = `{
  "description": {"text": "§b❄ Chillzone §b❄ §fModpack [1.0.1]"},
  "favicon": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg==",
  "forgeData": {
    "channels": [],
    "d": "AAF8AAADfAAABAAAAA==",
    "fmlNetworkVersion": 3,
    "mods": [],
    "truncated": false
  },
  "players": {"max": 20, "online": 0},
  "version": {"name": "1.20.1", "protocol": 763}
}`

// forge1165StatusJSON is the legacy (FML2) block shape, where the mod and
// channel lists are carried inline and visible to a status parser.
const forge1165StatusJSON = `{
  "description": {"text": "Forge 1.16.5"},
  "forgeData": {
    "channels": [
      {"res": "minecraft:register", "version": "FML2", "required": true},
      {"res": "forge:handshake", "version": "1.0", "required": true}
    ],
    "fmlNetworkVersion": 2,
    "mods": [
      {"modId": "minecraft", "modmarker": "1.16.5"},
      {"modId": "forge", "modmarker": "36.2.34"},
      {"modId": "jei", "modmarker": "7.7.1.152"}
    ],
    "truncated": false
  },
  "players": {"max": 20, "online": 3},
  "version": {"name": "1.16.5", "protocol": 754}
}`

// statusMember returns the raw top-level member "key" of a status JSON object.
func statusMember(t *testing.T, statusJSON, key string) json.RawMessage {
	t.Helper()
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(statusJSON), &doc))
	raw, ok := doc[key]
	require.Truef(t, ok, "status response lost the %q member", key)
	return raw
}

// TestServerPing_ForgeDataSurvivesReencode is the regression test for the
// defect: Gate re-encodes a backend's status response through ServerPing, so
// every member it does not model used to be dropped. Forge clients read
// forgeData from that response to decide whether the connection is modded.
func TestServerPing_ForgeDataSurvivesReencode(t *testing.T) {
	var p ServerPing
	require.NoError(t, json.Unmarshal([]byte(forge1201StatusJSON), &p))

	encoded, err := json.Marshal(&p)
	require.NoError(t, err)

	// The entire block must survive verbatim: on 1.20.1 the mod inventory lives
	// in the compressed "d" member, so a partial comparison would prove nothing.
	require.JSONEq(t, string(statusMember(t, forge1201StatusJSON, "forgeData")),
		string(statusMember(t, string(encoded), "forgeData")))

	// The modeled fields must still be re-encoded normally.
	require.JSONEq(t, `{"name":"1.20.1","protocol":763}`, string(statusMember(t, string(encoded), "version")))
	require.JSONEq(t, `{"max":20,"online":0}`, string(statusMember(t, string(encoded), "players")))
	require.JSONEq(t, `{"text":"§b❄ Chillzone §b❄ §fModpack [1.0.1]"}`,
		string(statusMember(t, string(encoded), "description")))
	require.JSONEq(t, `"data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=="`,
		string(statusMember(t, string(encoded), "favicon")))

	// And the decoded value must round-trip again (proxy hops re-encode more
	// than once on a two-hop path).
	var again ServerPing
	require.NoError(t, json.Unmarshal(encoded, &again))
	reencoded, err := json.Marshal(&again)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(reencoded))
}

// TestServerPing_ForgeLegacyBlockKeepsModAndChannelCounts covers the inline
// (FML2) block shape, where the advertised mod and channel lists are visible.
func TestServerPing_ForgeLegacyBlockKeepsModAndChannelCounts(t *testing.T) {
	var p ServerPing
	require.NoError(t, json.Unmarshal([]byte(forge1165StatusJSON), &p))

	encoded, err := json.Marshal(&p)
	require.NoError(t, err)

	forgeData := statusMember(t, string(encoded), "forgeData")
	var block struct {
		FMLNetworkVersion int `json:"fmlNetworkVersion"`
		Mods              []struct {
			ModID     string `json:"modId"`
			ModMarker string `json:"modmarker"`
		} `json:"mods"`
		Channels []struct {
			Res string `json:"res"`
		} `json:"channels"`
	}
	require.NoError(t, json.Unmarshal(forgeData, &block))
	require.Equal(t, 2, block.FMLNetworkVersion)
	require.Len(t, block.Mods, 3)
	require.Len(t, block.Channels, 2)
	require.Equal(t, "jei", block.Mods[2].ModID)
	require.Equal(t, "minecraft:register", block.Channels[0].Res)
}

// TestServerPing_PreservesUnmodeledMembers covers other status members Gate
// does not model: NeoForge's own modded block and vanilla's chat flags.
func TestServerPing_PreservesUnmodeledMembers(t *testing.T) {
	const statusJSON = `{
	  "description": {"text": "Neo"},
	  "enforcesSecureChat": true,
	  "neoforgeData": {"fmlNetworkVersion": 3, "d": "AAF8AAADfA==", "truncated": false, "mods": [], "channels": []},
	  "previewsChat": false,
	  "players": {"max": 10, "online": 1},
	  "version": {"name": "1.21.1", "protocol": 767}
	}`

	var p ServerPing
	require.NoError(t, json.Unmarshal([]byte(statusJSON), &p))
	encoded, err := json.Marshal(&p)
	require.NoError(t, err)

	require.JSONEq(t, `{"fmlNetworkVersion":3,"d":"AAF8AAADfA==","truncated":false,"mods":[],"channels":[]}`,
		string(statusMember(t, string(encoded), "neoforgeData")))
	require.JSONEq(t, `true`, string(statusMember(t, string(encoded), "enforcesSecureChat")))
	require.JSONEq(t, `false`, string(statusMember(t, string(encoded), "previewsChat")))
}

// TestServerPing_VanillaStatusGainsNoForgeBlock guards the other direction:
// a vanilla backend must not gain a modded block just because it was proxied.
func TestServerPing_VanillaStatusGainsNoForgeBlock(t *testing.T) {
	const statusJSON = `{
	  "description": {"text": "Vanilla"},
	  "players": {"max": 20, "online": 0},
	  "version": {"name": "1.20.1", "protocol": 763}
	}`

	var p ServerPing
	require.NoError(t, json.Unmarshal([]byte(statusJSON), &p))

	encoded, err := json.Marshal(&p)
	require.NoError(t, err)

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &doc))
	require.NotContains(t, doc, "forgeData")
	require.NotContains(t, doc, "modinfo")
	require.NotContains(t, doc, "neoforgeData")

	// A synthesized ping (no backend response behind it) must not gain one either.
	synthesized, err := json.Marshal(&ServerPing{
		Version:     Version{Protocol: 763, Name: "Gate"},
		Players:     &Players{Max: 1000},
		Description: &component.Text{Content: "Gate"},
	})
	require.NoError(t, err)
	require.NotContains(t, string(synthesized), "forgeData")
	require.NotContains(t, string(synthesized), "modinfo")
}

// TestServerPing_AnnounceForgeStillEmitsModInfo keeps the deliberate
// announceForge behaviour: legacy ModInfo is synthesized only when asked for,
// and no forgeData block is invented.
func TestServerPing_AnnounceForgeStillEmitsModInfo(t *testing.T) {
	p := &ServerPing{
		Version:     Version{Protocol: 763, Name: "Gate"},
		Description: &component.Text{Content: "Gate"},
		ModInfo:     &modinfo.ModInfo{Type: "FML"},
	}
	encoded, err := json.Marshal(p)
	require.NoError(t, err)

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &doc))
	require.JSONEq(t, `{"type":"FML","modList":null}`, string(doc["modinfo"]))
	require.NotContains(t, doc, "forgeData")
}
