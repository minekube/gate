package geyser

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

// The backendFloodgate link-promotion decision is otherwise invisible: the
// handshake triplet, the GeyserMC link API and every failure of it all end in
// the same XUID-derived identity. Support cases cannot tell "flag off", "XUID
// not linked" and "api.geysermc.org unreachable" apart, so each join must emit
// exactly ONE V(1) line naming the arm it took - and nothing that identifies
// the player.

const backendFloodgateLogPrefix = `"msg"="backend floodgate:`

var backendFloodgateIdentitySentinels = []string{
	"BedrockGuy",       // gamertag
	"987654321",        // XUID
	"JavaSteve",        // linked java_name
	linkedJavaUUIDStr,  // linked java UUID
	"00003ade68b1",     // Floodgate bedrock UUID for XUID 987654321
	"2535405290989773", // unrelated sentinel XUID
}

// backendFloodgateDecisionLines returns only the decision diagnostics.
func backendFloodgateDecisionLines(logs string) []string {
	var out []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, backendFloodgateLogPrefix) {
			out = append(out, line)
		}
	}
	return out
}

func newBackendFloodgateIntegration(t *testing.T, enabled bool, rt http.RoundTripper, verbosity int) (*Integration, func() string) {
	t.Helper()
	log, capture := capturingLoggerAt(verbosity)
	pm := NewProfileManager()
	pm.client = &http.Client{Transport: rt}
	return &Integration{
		log: log,
		config: &bconfig.Config{
			UsernameFormat:   ".%s",
			BackendFloodgate: bconfig.BackendFloodgate{Enabled: enabled},
		},
		profileManager: pm,
	}, capture
}

// TestOnGameProfileBackendFloodgateEmitsExactlyOneDecisionLine pins one bounded
// line per join per outcome: the handshake triplet, the API fallback, an
// unlinked XUID, a non-200, a transport failure and the flag being off must
// each be distinguishable, and never produce two lines.
func TestOnGameProfileBackendFloodgateEmitsExactlyOneDecisionLine(t *testing.T) {
	xuidURL := "https://api.geysermc.org/v2/link/bedrock/987654321"
	validLink := linkedJavaUsername + ";" + linkedJavaUUIDStr + ";" + linkedBedrockUUID().String()
	otherBedrockLink := linkedJavaUsername + ";" + linkedJavaUUIDStr + ";00000000-0000-0000-0000-0000000000ff"

	tests := []struct {
		name        string
		enabled     bool
		link        string
		rt          http.RoundTripper
		wantMsg     string
		wantField   string
		wantApplied bool
	}{
		{
			name:        "handshake triplet applied",
			enabled:     true,
			link:        validLink,
			rt:          failingRoundTripper{}, // must not be consulted
			wantMsg:     `"msg"="backend floodgate: linked identity applied"`,
			wantField:   `"source"="handshake"`,
			wantApplied: true,
		},
		{
			name:        "api fallback applied",
			enabled:     true,
			link:        "null",
			rt:          apiLinkRoundTripper{javaID: bug975LinkedJavaUUID, javaName: linkedJavaUsername},
			wantMsg:     `"msg"="backend floodgate: linked identity applied"`,
			wantField:   `"source"="api"`,
			wantApplied: true,
		},
		{
			name:      "unlinked xuid empty api body",
			enabled:   true,
			link:      "null",
			rt:        apiLinkRoundTripper{body: "{}"},
			wantMsg:   `"msg"="backend floodgate: no linked identity"`,
			wantField: `"reason"="not_linked"`,
		},
		{
			name:      "triplet for another connection and unlinked xuid",
			enabled:   true,
			link:      otherBedrockLink,
			rt:        apiLinkRoundTripper{body: "{}"},
			wantMsg:   `"msg"="backend floodgate: no linked identity"`,
			wantField: `"reason"="triplet_mismatch"`,
		},
		{
			name:      "non-200 link api",
			enabled:   true,
			link:      "null",
			rt:        apiLinkRoundTripper{statusCode: http.StatusInternalServerError},
			wantMsg:   `"msg"="backend floodgate: link lookup failed"`,
			wantField: `"cause"="status_500"`,
		},
		{
			name:      "link api unreachable",
			enabled:   true,
			link:      "null",
			rt:        apiLinkRoundTripper{err: &url.Error{Op: "Get", URL: xuidURL, Err: errors.New("dial tcp 1.2.3.4:443: connect: connection refused")}},
			wantMsg:   `"msg"="backend floodgate: link lookup failed"`,
			wantField: `"cause"="request_failed"`,
		},
		{
			name:      "link api timeout",
			enabled:   true,
			link:      "null",
			rt:        apiLinkRoundTripper{err: context.DeadlineExceeded},
			wantMsg:   `"msg"="backend floodgate: link lookup failed"`,
			wantField: `"cause"="timeout"`,
		},
		{
			name:      "backendFloodgate disabled",
			enabled:   false,
			link:      validLink,
			rt:        apiLinkRoundTripper{javaID: bug975LinkedJavaUUID, javaName: linkedJavaUsername},
			wantMsg:   `"msg"="backend floodgate: no linked identity"`,
			wantField: `"reason"="disabled"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i, capture := newBackendFloodgateIntegration(t, tt.enabled, tt.rt, 10)
			e := linkedGameProfileEvent(t, linkedBedrockData(tt.link))
			i.onGameProfile(e)

			logs := capture()
			lines := backendFloodgateDecisionLines(logs)
			require.Len(t, lines, 1, "exactly one backend floodgate decision line per join, got:\n%s", logs)
			require.Contains(t, lines[0], tt.wantMsg, "decision line must name the arm taken")
			require.Contains(t, lines[0], tt.wantField, "decision line must carry the bounded detail")

			for _, sentinel := range backendFloodgateIdentitySentinels {
				require.NotContains(t, logs, sentinel, "identity must never reach the log buffer")
			}

			// V(1) only: this path is hot and must stay out of default logs.
			quietIntegration, quietCapture := newBackendFloodgateIntegration(t, tt.enabled, tt.rt, 0)
			quietEvent := linkedGameProfileEvent(t, linkedBedrockData(tt.link))
			quietIntegration.onGameProfile(quietEvent)
			require.Empty(t, backendFloodgateDecisionLines(quietCapture()),
				"decision must be V(1), never an Info line")

			applied := e.GameProfile()
			expectedID, err := linkedBedrockData(tt.link).JavaUuid()
			require.NoError(t, err)
			if tt.wantApplied {
				wantUUID, err := uuid.Parse(linkedJavaUUIDStr)
				require.NoError(t, err)
				require.Equal(t, wantUUID, applied.ID, "linked Java identity must be applied")
				require.Equal(t, linkedJavaUsername, applied.Name)
			} else {
				require.Equal(t, expectedID, applied.ID, "XUID-derived identity must be kept")
				require.Equal(t, "_BedrockGuy", applied.Name)
			}
		})
	}
}

// TestBackendFloodgateDecisionLogCarriesNoIdentity is the privacy half of the
// decision line: the failure category is a bounded allow-list (never the
// GeyserMC URL, which embeds the XUID), and the applied arms name only the
// source.
func TestBackendFloodgateDecisionLogCarriesNoIdentity(t *testing.T) {
	const sentinelXUID = int64(2535405290989773)
	const sentinelGamertag = "SentinelGamertag"
	const sentinelJavaName = "SentinelJavaName"
	const sentinelJavaUUID = "99999999-8888-7777-6666-555555555555"

	data := &floodgate.BedrockData{Username: sentinelGamertag, Xuid: sentinelXUID, LinkedPlayer: "null"}
	xuidURL := "https://api.geysermc.org/v2/link/bedrock/2535405290989773"

	arms := map[string]http.RoundTripper{
		"api applied":       apiLinkRoundTripper{javaID: sentinelJavaUUID, javaName: sentinelJavaName},
		"unlinked":          apiLinkRoundTripper{body: "{}"},
		"status failure":    apiLinkRoundTripper{statusCode: http.StatusForbidden},
		"transport failure": apiLinkRoundTripper{err: &url.Error{Op: "Get", URL: xuidURL, Err: errors.New("boom")}},
	}
	for name, rt := range arms {
		t.Run(name, func(t *testing.T) {
			i, capture := newBackendFloodgateIntegration(t, true, rt, 10)
			geyserConn := &GeyserConnection{BedrockData: &floodgate.BedrockData{
				Username: data.Username, Xuid: data.Xuid, LinkedPlayer: data.LinkedPlayer,
			}}
			geyserConn.Context = withBedrockContext(context.Background(), geyserConn)
			e := proxy.NewGameProfileRequestEvent(
				&fakeInbound{ctx: geyserConn.Context},
				profile.GameProfile{Name: data.Username},
				false,
			)
			i.onGameProfile(e)

			require.Len(t, backendFloodgateDecisionLines(capture()), 1)
			logs := capture()
			for _, sentinel := range []string{
				sentinelGamertag,
				"2535405290989773",
				sentinelJavaName,
				sentinelJavaUUID,
			} {
				require.NotContains(t, logs, sentinel, "identity must never reach the log buffer")
			}
		})
	}
}
