package geyser

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	bedrockconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/netutil"
)

var _ proxy.BackendHandshakeAddresser = (*Integration)(nil)

type contextOnlyPlayer struct {
	proxy.Player
	ctx context.Context
}

func (p contextOnlyPlayer) Context() context.Context {
	return p.ctx
}

type backendHandshakeTarget struct {
	info proxy.ServerInfo
}

func (t backendHandshakeTarget) ServerInfo() proxy.ServerInfo {
	return t.info
}

func (t backendHandshakeTarget) Players() proxy.Players {
	return nil
}

func TestBackendHandshakeAddrEmitsFloodgateDataForAllowedBedrockTarget(t *testing.T) {
	fg := newTestFloodgate(t)
	bedrockData := testBedrockData()
	ctx := withBedrockContext(context.Background(), &GeyserConnection{
		BedrockData:  bedrockData,
		OriginalHost: "original.example.org:19132",
	})
	integration := &Integration{
		config: &bedrockconfig.Config{
			BackendFloodgate: bedrockconfig.BackendFloodgate{
				Enabled:        true,
				AllowedServers: []string{"lobby"},
			},
		},
		floodgate: fg,
	}

	got, err := integration.BackendHandshakeAddr("clean.example.org", contextOnlyPlayer{ctx: ctx}, testBackendTarget("Lobby"))

	require.NoError(t, err)
	original, decoded, err := fg.ReadHostname(got)
	require.NoError(t, err)
	require.Equal(t, "clean.example.org", original)
	require.Equal(t, *bedrockData, *decoded)
}

func TestBackendHandshakeAddrLeavesNonAllowedTargetClean(t *testing.T) {
	fg := newTestFloodgate(t)
	ctx := withBedrockContext(context.Background(), &GeyserConnection{
		BedrockData:  testBedrockData(),
		OriginalHost: "original.example.org:19132",
	})
	integration := &Integration{
		config: &bedrockconfig.Config{
			BackendFloodgate: bedrockconfig.BackendFloodgate{
				Enabled:        true,
				AllowedServers: []string{"lobby"},
			},
		},
		floodgate: fg,
	}

	got, err := integration.BackendHandshakeAddr("clean.example.org", contextOnlyPlayer{ctx: ctx}, testBackendTarget("survival"))

	require.NoError(t, err)
	require.Equal(t, "clean.example.org", got)
}

func TestBackendHandshakeAddrRejectsSpoofedJavaFloodgateHostnameForAllowedTarget(t *testing.T) {
	integration := &Integration{
		config: &bedrockconfig.Config{
			BackendFloodgate: bedrockconfig.BackendFloodgate{
				Enabled:        true,
				AllowedServers: []string{"lobby"},
			},
		},
		floodgate: newTestFloodgate(t),
	}

	_, err := integration.BackendHandshakeAddr("clean.example.org\x00spoofed", contextOnlyPlayer{ctx: context.Background()}, testBackendTarget("lobby"))

	require.Error(t, err)
}

func TestBackendHandshakeAddrFailsClosedForAllowedBedrockTarget(t *testing.T) {
	fg := newTestFloodgate(t)
	bedrockData := testBedrockData()
	bedrockData.Username = "Bedrock\x00Admin"
	ctx := withBedrockContext(context.Background(), &GeyserConnection{
		BedrockData: bedrockData,
	})
	integration := &Integration{
		config: &bedrockconfig.Config{
			BackendFloodgate: bedrockconfig.BackendFloodgate{
				Enabled:        true,
				AllowedServers: []string{"lobby"},
			},
		},
		floodgate: fg,
	}

	_, err := integration.BackendHandshakeAddr("clean.example.org", contextOnlyPlayer{ctx: ctx}, testBackendTarget("lobby"))

	require.Error(t, err)
}

func TestNewIntegrationRegistersAndStopsBackendHandshakeAddresserWhenEnabled(t *testing.T) {
	keyPath := writeTestFloodgateKey(t)
	p := new(proxy.Proxy)
	integration, err := NewIntegration(context.Background(), p, &bedrockconfig.Config{
		FloodgateKeyPath: keyPath,
		GeyserListenAddr: "127.0.0.1:0",
		BackendFloodgate: bedrockconfig.BackendFloodgate{
			Enabled:        true,
			AllowedServers: []string{"lobby"},
		},
	})
	require.NoError(t, err)
	require.True(t, backendHandshakeAddresserRegistered(p))

	integration.Stop()

	require.False(t, backendHandshakeAddresserRegistered(p))
}

func TestNewIntegrationDoesNotRegisterBackendHandshakeAddresserWhenDisabled(t *testing.T) {
	keyPath := writeTestFloodgateKey(t)
	p := new(proxy.Proxy)
	_, err := NewIntegration(context.Background(), p, &bedrockconfig.Config{
		FloodgateKeyPath: keyPath,
		GeyserListenAddr: "127.0.0.1:0",
	})
	require.NoError(t, err)

	require.False(t, backendHandshakeAddresserRegistered(p))
}

func newTestFloodgate(t *testing.T) *floodgate.Floodgate {
	t.Helper()
	fg, err := floodgate.NewFloodgate(bytes.Repeat([]byte{0x24}, 16))
	require.NoError(t, err)
	return fg
}

func testBedrockData() *floodgate.BedrockData {
	return &floodgate.BedrockData{
		Version:      "1",
		Username:     "BedrockPlayer",
		Xuid:         987654321,
		DeviceOS:     floodgate.DeviceOSAndroid,
		Language:     "en_US",
		UIProfile:    1,
		InputMode:    1,
		IP:           "198.51.100.8",
		LinkedPlayer: "",
		Proxy:        false,
		SubscribeID:  "sub",
		VerifyCode:   "code",
	}
}

func testBackendTarget(name string) proxy.RegisteredServer {
	return backendHandshakeTarget{info: proxy.NewServerInfo(name, netutil.NewAddr("127.0.0.1:25566", "tcp"))}
}

func writeTestFloodgateKey(t *testing.T) string {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), "floodgate.key")
	require.NoError(t, os.WriteFile(keyPath, bytes.Repeat([]byte{0x24}, 16), 0o600))
	return keyPath
}

func backendHandshakeAddresserRegistered(p *proxy.Proxy) bool {
	field := reflect.ValueOf(p).Elem().FieldByName("backendHandshakeAddresser")
	return !field.IsNil()
}
