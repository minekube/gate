package packet

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

func TestJoinGameMinecraft262OnlineModeRoundTrip(t *testing.T) {
	levelName := "minecraft:overworld"
	original := &JoinGame{
		EntityID:           1,
		Hardcore:           true,
		LevelNames:         []string{"minecraft:overworld"},
		MaxPlayers:         100,
		ViewDistance:       12,
		SimulationDistance: 10,
		ReducedDebugInfo:   true,
		ShowRespawnScreen:  true,
		DoLimitedCrafting:  true,
		Dimension:          0,
		DimensionInfo: &DimensionInfo{
			LevelName: &levelName,
		},
		PartialHashedSeed:  1234,
		Gamemode:           1,
		PreviousGamemode:   -1,
		LastDeathPosition:  &DeathPosition{Key: "minecraft:overworld", Value: 42},
		SeaLevel:           63,
		PortalCooldown:     5,
		OnlineMode:         true,
		EnforcesSecureChat: true,
	}
	ctx := &proto.PacketContext{Direction: proto.ClientBound, Protocol: version.Minecraft_26_2.Protocol}

	var buf bytes.Buffer
	require.NoError(t, original.Encode(ctx, &buf))
	require.True(t, bytes.HasSuffix(buf.Bytes(), []byte{
		0x05, // portal cooldown
		0x3f, // sea level
		0x01, // online mode
		0x01, // enforces secure chat
	}))

	var decoded JoinGame
	require.NoError(t, decoded.Decode(ctx, &buf))
	require.True(t, decoded.OnlineMode)
	require.True(t, decoded.EnforcesSecureChat)
	require.Equal(t, 0, buf.Len())
}

func TestServerLoginSuccessMinecraft262SessionIDRoundTrip(t *testing.T) {
	sessionID := uuid.New()
	original := &ServerLoginSuccess{
		UUID:      testUUID,
		Username:  "Robin",
		SessionID: sessionID,
	}
	ctx := &proto.PacketContext{Direction: proto.ClientBound, Protocol: version.Minecraft_26_2.Protocol}

	var buf bytes.Buffer
	require.NoError(t, original.Encode(ctx, &buf))

	var decoded ServerLoginSuccess
	require.NoError(t, decoded.Decode(ctx, &buf))
	require.Equal(t, sessionID, decoded.SessionID)
	require.Equal(t, 0, buf.Len())
}
