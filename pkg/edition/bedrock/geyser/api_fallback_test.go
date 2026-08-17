package geyser

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

// apiLinkRoundTripper answers the GeyserMC link API (GET /v2/link/bedrock/<xuid>)
// with a canned LinkedAccountResult, or fails the request when errOnCall is set.
type apiLinkRoundTripper struct {
	javaID   string
	javaName string
	err      error
}

func (rt apiLinkRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if rt.err != nil {
		return nil, rt.err
	}
	if !strings.Contains(r.URL.Path, "/v2/link/bedrock/") {
		return nil, errRoundTripUnexpectedPath(r.URL.Path)
	}
	body := `{"bedrock_id":987654321,"java_id":"` + rt.javaID + `","java_name":"` + rt.javaName + `","last_name_update":0}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

var errRoundTripUnexpectedPath = func(p string) error { return errAPIUnexpectedPath{p: p} }

type errAPIUnexpectedPath struct{ p string }

func (e errAPIUnexpectedPath) Error() string { return "unexpected api path: " + e.p }

// TestBug975LinkedIdentityFallsBackToGeyserApiWhenHandshakeHasNoTriplet is the
// failing regression test for the standalone Geyser -> Gate topology
// (minekube/gate#975): standalone Geyser never sends the linked-player
// triplet (field 8 is the literal "null"), so Gate must fall back to the
// official GeyserMC global link API (the same source the backend Floodgate
// plugin itself uses via GlobalPlayerLinking, enable-global-linking default
// true) to resolve the linked Java identity -- gated on backendFloodgate.
func TestBug975LinkedIdentityFallsBackToGeyserApiWhenHandshakeHasNoTriplet(t *testing.T) {
	bedrockData := &floodgate.BedrockData{
		Username:     "BedrockGuy",
		Xuid:         987654321,
		LinkedPlayer: "null", // standalone Geyser emits no link
	}

	log, capture := capturingLogger()
	pm := NewProfileManager()
	pm.client = &http.Client{Transport: apiLinkRoundTripper{
		javaID:   bug975LinkedJavaUUID,
		javaName: "JavaSteve",
	}}
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
		"linked Java UUID must be applied via GeyserMC link API fallback (gate#975)")
	require.Equal(t, "JavaSteve", applied.Name,
		"linked Java name must be applied via GeyserMC link API fallback (gate#975)")

	// Privacy: no raw identity (linked Java username, XUID, UUIDs) may reach logs.
	logs := capture()
	require.NotContains(t, logs, "JavaSteve")
	require.NotContains(t, logs, "987654321")
	require.NotContains(t, logs, bug975LinkedJavaUUID)
	require.NotContains(t, logs, bug975BedrockUUID)
}

// TestOnGameProfileApiFallbackFailClosedKeepsXuidIdentity verifies the API
// fallback is fail-closed: when the GeyserMC link API errors, the player keeps
// the XUID-derived identity instead of being promoted or dropped.
func TestOnGameProfileApiFallbackFailClosedKeepsXuidIdentity(t *testing.T) {
	bedrockData := &floodgate.BedrockData{
		Username:     "BedrockGuy",
		Xuid:         987654321,
		LinkedPlayer: "", // no triplet
	}

	log, _ := capturingLogger()
	pm := NewProfileManager()
	pm.client = &http.Client{Transport: apiLinkRoundTripper{
		err: context.DeadlineExceeded,
	}}
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
	expectedID, err := bedrockData.JavaUuid()
	require.NoError(t, err)
	require.Equal(t, expectedID, applied.ID, "API failure must keep the XUID-derived identity")
	require.Equal(t, "_BedrockGuy", applied.Name)
}

// TestOnGameProfileApiFallbackGatedOnBackendFloodgate verifies the GeyserMC
// link API is only consulted when the operator opted into the backendFloodgate
// trust boundary; with the feature disabled, no API call happens and the
// XUID-derived identity stays (the #961 posture).
func TestOnGameProfileApiFallbackGatedOnBackendFloodgate(t *testing.T) {
	bedrockData := &floodgate.BedrockData{
		Username:     "BedrockGuy",
		Xuid:         987654321,
		LinkedPlayer: "",
	}

	log, _ := capturingLogger()
	pm := NewProfileManager()
	pm.client = &http.Client{Transport: apiLinkRoundTripper{
		javaID:   bug975LinkedJavaUUID,
		javaName: "JavaSteve",
	}}
	integration := &Integration{
		log: log,
		config: &bconfig.Config{
			UsernameFormat: ".%s",
			BackendFloodgate: bconfig.BackendFloodgate{
				Enabled: false,
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
	expectedID, err := bedrockData.JavaUuid()
	require.NoError(t, err)
	require.Equal(t, expectedID, applied.ID, "backendFloodgate disabled must keep XUID-derived identity")
	require.Equal(t, "_BedrockGuy", applied.Name)
}
