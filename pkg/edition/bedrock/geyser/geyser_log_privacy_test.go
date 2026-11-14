package geyser

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/require"

	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/proto"
)

type fakeInbound struct{ ctx context.Context }

func (f *fakeInbound) Protocol() proto.Protocol                { return 0 }
func (f *fakeInbound) VirtualHost() net.Addr                   { return nil }
func (f *fakeInbound) HandshakeIntent() packet.HandshakeIntent { return packet.LoginHandshakeIntent }
func (f *fakeInbound) RemoteAddr() net.Addr                    { return nil }
func (f *fakeInbound) Active() bool                            { return true }
func (f *fakeInbound) Context() context.Context                { return f.ctx }

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network disabled in test")
}

func capturingLogger() (logr.Logger, func() string) {
	return capturingLoggerAt(10)
}

// capturingLoggerAt returns a logger whose verbosity is fixed at v, so a test
// can prove that a diagnostic is emitted at V(1) and never at Info.
func capturingLoggerAt(v int) (logr.Logger, func() string) {
	var mu sync.Mutex
	var lines []string
	log := funcr.New(func(prefix, args string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, prefix+" "+args)
	}, funcr.Options{Verbosity: v})
	return log, func() string {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(lines, "\n")
	}
}

func TestOnGameProfileLogsNoIdentityAndAppliesNoLinkHint(t *testing.T) {
	const (
		sentinelUsername = "SentinelGamertag"
		sentinelXUID     = int64(2535405290989773)
	)
	log, capture := capturingLogger()

	pm := NewProfileManager()
	pm.client = &http.Client{Transport: failingRoundTripper{}}

	i := &Integration{
		log:            log,
		config:         &bconfig.Config{UsernameFormat: ".%s"},
		profileManager: pm,
	}

	bedrockData := &floodgate.BedrockData{
		Username: sentinelUsername,
		Xuid:     sentinelXUID,
	}
	geyserConn := &GeyserConnection{BedrockData: bedrockData}
	geyserConn.Context = withBedrockContext(context.Background(), geyserConn)

	e := proxy.NewGameProfileRequestEvent(
		&fakeInbound{ctx: geyserConn.Context},
		profile.GameProfile{Name: sentinelUsername},
		false,
	)
	i.onGameProfile(e)

	// Exactly one profile is applied: the XUID-derived offline identity.
	// The GeyserMC link lookup is only consulted for identity promotion when
	// the operator explicitly opted in via backendFloodgate (default off),
	// so with the default config the profile can never be promoted to a
	// linked Java UUID or name here.
	applied := e.GameProfile()
	require.Equal(t, "_SentinelGamerta", applied.Name)
	expectedID, err := bedrockData.JavaUuid()
	require.NoError(t, err)
	require.Equal(t, expectedID, applied.ID)

	logs := capture()
	require.NotContains(t, logs, sentinelUsername)
	require.NotContains(t, logs, "2535405290989773")
}

// TestOnGameProfileBackendFloodgateDecisionLineIsPrivacySafe extends the
// privacy invariant to the three link-promotion arms the operator now sees:
// the handshake triplet, the GeyserMC link API and an API failure. Each arm
// emits its bounded decision line at V(1), and no XUID, gamertag, java_name
// or UUID may reach the log buffer with it.
func TestOnGameProfileBackendFloodgateDecisionLineIsPrivacySafe(t *testing.T) {
	const (
		gamertag     = "PrivacyGamertag"
		xuid         = int64(2535405290989773)
		javaName     = "PrivacyJavaName"
		javaUUID     = "99999999-8888-7777-6666-555555555555"
		bedrockUUID  = "00000000-0000-0000-0000-0009003c0009"
		handshakeArm = "handshake"
	)

	link := javaName + ";" + javaUUID + ";" + (&floodgate.BedrockData{Xuid: xuid}).FloodgateJavaUuid().String()

	tests := []struct {
		name      string
		link      string
		rt        http.RoundTripper
		wantField string
	}{
		{
			name:      handshakeArm,
			link:      link,
			rt:        failingRoundTripper{},
			wantField: `"source"="handshake"`,
		},
		{
			name:      "api",
			link:      "null",
			rt:        apiLinkRoundTripper{javaID: javaUUID, javaName: javaName},
			wantField: `"source"="api"`,
		},
		{
			name:      "api failure",
			link:      "null",
			rt:        apiLinkRoundTripper{err: errors.New("network disabled in test")},
			wantField: `"cause"="request_failed"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i, capture := newBackendFloodgateIntegration(t, true, tt.rt, 10)
			geyserConn := &GeyserConnection{BedrockData: &floodgate.BedrockData{
				Username: gamertag, Xuid: xuid, LinkedPlayer: tt.link,
			}}
			geyserConn.Context = withBedrockContext(context.Background(), geyserConn)
			e := proxy.NewGameProfileRequestEvent(
				&fakeInbound{ctx: geyserConn.Context},
				profile.GameProfile{Name: gamertag},
				false,
			)
			i.onGameProfile(e)

			logs := capture()
			lines := backendFloodgateDecisionLines(logs)
			require.Len(t, lines, 1, "one decision line per join:\n%s", logs)
			require.Contains(t, lines[0], tt.wantField)

			for _, sentinel := range []string{gamertag, "2535405290989773", javaName, javaUUID, bedrockUUID} {
				require.NotContains(t, logs, sentinel, "identity must never reach the log buffer")
			}
		})
	}
}
