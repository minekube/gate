package proxy

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
)

// rawClientHandshake returns the exact wire bytes a client sends for a login
// handshake declaring the given host and port. It uses the primitive protocol
// writers only, so the fixture does not depend on packet.Handshake's own port
// handling (that is what is under test here).
func rawClientHandshake(t *testing.T, host string, port uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, util.WriteVarInt(&buf, int(version.Minecraft_1_20_2.Protocol)))
	require.NoError(t, util.WriteString(&buf, host))
	require.NoError(t, util.WriteUint16(&buf, port))
	require.NoError(t, util.WriteVarInt(&buf, 2)) // login
	return buf.Bytes()
}

// TestHandshakeVirtualHostPortIsUnsigned is the routing-seam regression test:
// the client-declared port is unsigned on the wire, so a declared port >= 32768
// must reach the virtual host as the unsigned port (or a documented fallback) -
// never as a negative number such as "play.example.com:-1", which would break
// forced-host matching and virtual host forwarding.
// See https://discord.com/channels/633708750032863232/1550961184448979024
func TestHandshakeVirtualHostPortIsUnsigned(t *testing.T) {
	tests := []struct {
		name string
		wire uint16
		want string
	}{
		{"default client port", 25565, "play.example.com:25565"},
		{"max signed int16", 32767, "play.example.com:32767"},
		{"first unsigned-only port", 32768, "play.example.com:32768"},
		{"max unsigned 16-bit", 65535, "play.example.com:65535"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handshake packet.Handshake
			require.NoError(t, handshake.Decode(nil, bytes.NewReader(rawClientHandshake(t, "play.example.com", tt.wire))))

			vHost := handshakeVirtualHost(&handshake, "tcp").String()
			require.Equal(t, tt.want, vHost)
			require.NotContains(t, vHost, ":-", "a negative port must never reach the routing virtual host")
		})
	}
}

// TestHandshakeVirtualHostKeepsVirtualHostSuffix pins that the helper keeps
// formatting the whole client-declared address (including the \0-separated
// Forge/forwarding suffix) exactly as before.
func TestHandshakeVirtualHostKeepsVirtualHostSuffix(t *testing.T) {
	handshake := &packet.Handshake{
		ServerAddress: "play.example.com\x00FML3\x00",
		Port:          25565,
	}
	require.Equal(t, "play.example.com\x00FML3\x00:25565", handshakeVirtualHost(handshake, "tcp").String())
}
