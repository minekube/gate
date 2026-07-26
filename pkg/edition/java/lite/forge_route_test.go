package lite

import (
	"errors"
	"net"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/errs"
)

// routeTestConn exposes the underlying connection, which is all findRoute
// needs from a client.
type routeTestConn struct {
	netmc.MinecraftConn
	conn net.Conn
}

func (c *routeTestConn) Conn() net.Conn { return c.conn }

// TestFindRouteMatchesForgeHandshakeHost covers Gate Lite behind another proxy:
// a Forge client appends the token to its virtual host, so the token must not
// stop the route from matching. An address carrying only the token, as Gate
// itself produced before the fix, has no host left to match on.
func TestFindRouteMatchesForgeHandshakeHost(t *testing.T) {
	const backendAddr = "127.0.0.1:25566"
	routes := []config.Route{{
		Host:    []string{"play.example.org"},
		Backend: []string{backendAddr},
	}}

	tests := []struct {
		name          string
		serverAddress string
		wantRoute     bool
	}{
		{name: "vanilla", serverAddress: "play.example.org", wantRoute: true},
		{name: "modern forge", serverAddress: "play.example.org\x00FORGE", wantRoute: true},
		{name: "modern forge with NAT version", serverAddress: "play.example.org\x00FORGE2", wantRoute: true},
		{name: "FML3 forge", serverAddress: "play.example.org\x00FML3\x00", wantRoute: true},
		{name: "token without host", serverAddress: "\x00FORGE", wantRoute: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() {
				_ = client.Close()
				_ = server.Close()
			})

			log, _, route, nextBackend, err := findRoute(
				routes,
				testr.New(t),
				&routeTestConn{conn: client},
				&packet.Handshake{
					ServerAddress:   tt.serverAddress,
					ProtocolVersion: int(version.Minecraft_1_20_2.Protocol),
				},
				NewStrategyManager(),
			)

			if !tt.wantRoute {
				require.Error(t, err)
				require.Nil(t, route)
				// Forward logs this for players at the default verbosity, only the
				// error carries the verbosity that keeps status pings quiet.
				require.Zero(t, log.GetV(), "no route must stay visible for player connections")
				var verbosityErr *errs.VerbosityError
				require.True(t, errors.As(err, &verbosityErr))
				require.Equal(t, 1, verbosityErr.Verbosity)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, route)
			got, _, ok := nextBackend()
			require.True(t, ok)
			require.Equal(t, backendAddr, got)
		})
	}
}
