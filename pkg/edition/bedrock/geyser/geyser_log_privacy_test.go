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
	var mu sync.Mutex
	var lines []string
	log := funcr.New(func(prefix, args string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, prefix+" "+args)
	}, funcr.Options{Verbosity: 10})
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
	// The unauthenticated GeyserMC link lookup no longer exists, so the
	// profile can never be promoted to a linked Java UUID or name here.
	applied := e.GameProfile()
	require.Equal(t, ".SentinelGamertag", applied.Name)
	expectedID, err := bedrockData.JavaUuid()
	require.NoError(t, err)
	require.Equal(t, expectedID, applied.ID)

	logs := capture()
	require.NotContains(t, logs, sentinelUsername)
	require.NotContains(t, logs, "2535405290989773")
}
