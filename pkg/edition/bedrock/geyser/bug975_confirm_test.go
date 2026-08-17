package geyser

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

// bug975LinkedJavaUUID is the linked Java account UUID the Floodgate handshake
// triplet claims for the Bedrock player (gate#975).
const bug975LinkedJavaUUID = "11111111-2222-3333-4444-555555555555"

// bug975BedrockUUID is Floodgate's bedrock-side UUID for XUID 987654321
// (new UUID(0, xuid)); the triplet's third part must equal this for the link
// to belong to the connection.
const bug975BedrockUUID = "00000000-0000-0000-0000-00003ade68b1"

// TestBug975LinkedFloodgateIdentityAppliedWhenBackendFloodgateEnabled is the
// failing regression test for minekube/gate#975: a Bedrock player whose
// Floodgate handshake carries a linked Java account (field 8 triplet
// "javaUsername;javaUUID;bedrockUUID") must get the linked Java UUID/name as
// their game profile when the operator opted into the backendFloodgate trust
// boundary -- not the XUID-derived UUID.
func TestBug975LinkedFloodgateIdentityAppliedWhenBackendFloodgateEnabled(t *testing.T) {
	// Build the Floodgate handshake triplet exactly as a Floodgate-enabled
	// proxy re-emits it after its own link lookup.
	validLink := "JavaSteve;" + bug975LinkedJavaUUID + ";" + bug975BedrockUUID
	bedrockData := &floodgate.BedrockData{
		Username:     "BedrockGuy",
		Xuid:         987654321,
		LinkedPlayer: validLink,
	}

	log, capture := capturingLogger()
	pm := NewProfileManager()
	pm.client = &http.Client{Transport: failingRoundTripper{}}
	integration := &Integration{
		log: log,
		config: &bconfig.Config{
			UsernameFormat: ".%s",
			BackendFloodgate: bconfig.BackendFloodgate{
				Enabled: true,
			},
		},
		profileManager: pm,
	}

	geyserConn := &GeyserConnection{BedrockData: bedrockData}
	geyserConn.Context = withBedrockContext(context.Background(), geyserConn)
	e := proxy.NewGameProfileRequestEvent(
		&fakeInbound{ctx: geyserConn.Context},
		profile.GameProfile{Name: bedrockData.Username},
		false,
	)

	integration.onGameProfile(e)

	applied := e.GameProfile()
	wantUUID, err := uuid.Parse(bug975LinkedJavaUUID)
	require.NoError(t, err)
	require.Equal(t, wantUUID, applied.ID,
		"linked Java UUID must be applied to the Bedrock profile (gate#975)")
	require.Equal(t, "JavaSteve", applied.Name,
		"linked Java name must be applied to the Bedrock profile (gate#975)")

	// Privacy: no raw identity may reach the logs.
	logs := capture()
	require.NotContains(t, logs, "JavaSteve")
	require.NotContains(t, logs, "987654321")
	require.NotContains(t, logs, bug975LinkedJavaUUID)
	require.NotContains(t, logs, bug975BedrockUUID)
}
