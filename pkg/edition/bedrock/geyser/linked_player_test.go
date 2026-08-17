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

const (
	linkedJavaUsername = "JavaSteve"
	linkedJavaUUIDStr  = "11111111-2222-3333-4444-555555555555"
	linkedBedrockXUID  = int64(987654321)
)

func linkedJavaUUID(t *testing.T) uuid.UUID {
	t.Helper()
	u, err := uuid.Parse(linkedJavaUUIDStr)
	require.NoError(t, err)
	return u
}

// linkedBedrockUUID is Floodgate's bedrock side UUID for linkedBedrockXUID
// (new UUID(0, xuid)), the value Floodgate stores in a LinkedPlayer triplet.
func linkedBedrockUUID() uuid.UUID {
	return (&floodgate.BedrockData{Xuid: linkedBedrockXUID}).FloodgateJavaUuid()
}

func linkedBedrockData(link string) *floodgate.BedrockData {
	return &floodgate.BedrockData{
		Username:     "BedrockGuy",
		Xuid:         linkedBedrockXUID,
		LinkedPlayer: link,
	}
}

func linkedTestIntegration(t *testing.T, backendFloodgateEnabled bool) (*Integration, func() string) {
	t.Helper()
	log, capture := capturingLogger()
	pm := NewProfileManager()
	pm.client = &http.Client{Transport: failingRoundTripper{}}
	return &Integration{
		log:            log,
		config:         &bconfig.Config{UsernameFormat: ".%s", BackendFloodgate: bconfig.BackendFloodgate{Enabled: backendFloodgateEnabled}},
		profileManager: pm,
	}, capture
}

func linkedGameProfileEvent(t *testing.T, bedrockData *floodgate.BedrockData) *proxy.GameProfileRequestEvent {
	t.Helper()
	geyserConn := &GeyserConnection{BedrockData: bedrockData}
	geyserConn.Context = withBedrockContext(context.Background(), geyserConn)
	return proxy.NewGameProfileRequestEvent(
		&fakeInbound{ctx: geyserConn.Context},
		profile.GameProfile{Name: bedrockData.Username},
		false,
	)
}

func TestOnGameProfileAppliesLinkedIdentityWhenBackendFloodgateEnabledAndMatching(t *testing.T) {
	validLink := linkedJavaUsername + ";" + linkedJavaUUIDStr + ";" + linkedBedrockUUID().String()
	i, capture := linkedTestIntegration(t, true)

	e := linkedGameProfileEvent(t, linkedBedrockData(validLink))
	i.onGameProfile(e)

	applied := e.GameProfile()
	require.Equal(t, linkedJavaUUID(t), applied.ID)
	require.Equal(t, "JavaSteve", applied.Name)

	// No raw identity (linked Java username, XUID, UUIDs) may reach the logs.
	logs := capture()
	require.NotContains(t, logs, linkedJavaUsername)
	require.NotContains(t, logs, "987654321")
	require.NotContains(t, logs, linkedJavaUUIDStr)
	require.NotContains(t, logs, linkedBedrockUUID().String())
}

func TestOnGameProfileKeepsXuidIdentityWhenBackendFloodgateDisabled(t *testing.T) {
	validLink := linkedJavaUsername + ";" + linkedJavaUUIDStr + ";" + linkedBedrockUUID().String()
	i, _ := linkedTestIntegration(t, false)

	e := linkedGameProfileEvent(t, linkedBedrockData(validLink))
	i.onGameProfile(e)

	applied := e.GameProfile()
	expectedID, err := linkedBedrockData(validLink).JavaUuid()
	require.NoError(t, err)
	require.Equal(t, expectedID, applied.ID)
	require.Equal(t, "_BedrockGuy", applied.Name)
}

func TestOnGameProfileRejectsLinkedIdentityOnBedrockUuidMismatch(t *testing.T) {
	// The triplet's bedrock UUID belongs to a different Bedrock connection:
	// the link must never be promoted onto this one.
	otherBedrockUUID, err := uuid.Parse("00000000-0000-0000-0000-0000000000ff")
	require.NoError(t, err)
	link := linkedJavaUsername + ";" + linkedJavaUUIDStr + ";" + otherBedrockUUID.String()
	i, _ := linkedTestIntegration(t, true)

	e := linkedGameProfileEvent(t, linkedBedrockData(link))
	i.onGameProfile(e)

	applied := e.GameProfile()
	expectedID, err := linkedBedrockData(link).JavaUuid()
	require.NoError(t, err)
	require.Equal(t, expectedID, applied.ID)
	require.Equal(t, "_BedrockGuy", applied.Name)
}

func TestOnGameProfileKeepsXuidIdentityForAbsentOrMalformedLink(t *testing.T) {
	for _, link := range []string{"", "null", "two;parts", "a;b;c;d", "user;bad-uuid;" + linkedBedrockUUID().String()} {
		i, _ := linkedTestIntegration(t, true)

		e := linkedGameProfileEvent(t, linkedBedrockData(link))
		i.onGameProfile(e)

		applied := e.GameProfile()
		expectedID, err := linkedBedrockData(link).JavaUuid()
		require.NoError(t, err)
		require.Equal(t, expectedID, applied.ID, "link %q must keep the XUID-derived identity", link)
		require.Equal(t, "_BedrockGuy", applied.Name, "link %q must keep the Bedrock name", link)
	}
}
