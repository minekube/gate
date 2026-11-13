package packet

import (
	"bytes"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
)

// The handshake `port` field is an UNSIGNED 16-bit value on the wire
// (wiki.vg -> Protocol -> Handshaking -> Handshake). Decoding it as a signed
// int16 makes every port >= 32768 arrive negative (65535 -> -1), which then
// leaks into the virtual host string used for routing.
// See https://discord.com/channels/633708750032863232/1550961184448979024

// handshakeWireBytes builds the raw wire form of a client handshake using the
// primitive protocol writers only, so the fixture is independent of
// Handshake.Encode's own port handling.
func handshakeWireBytes(t *testing.T, host string, port uint16, nextStatus int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, util.WriteVarInt(&buf, int(version.Minecraft_1_20_2.Protocol)))
	require.NoError(t, util.WriteString(&buf, host))
	require.NoError(t, util.WriteUint16(&buf, port))
	require.NoError(t, util.WriteVarInt(&buf, nextStatus))
	return buf.Bytes()
}

// TestHandshakeDecodePortIsUnsigned proves the wire field is read unsigned.
// On the signed-int16 implementation the two upper cases fail
// (32768 -> -32768, 65535 -> -1).
func TestHandshakeDecodePortIsUnsigned(t *testing.T) {
	tests := []struct {
		name string
		wire uint16
		want int
	}{
		{"zero", 0, 0},
		{"default client port", 25565, 25565},
		{"max signed int16", 32767, 32767},
		{"first unsigned-only port", 32768, 32768},
		{"max unsigned 16-bit", 65535, 65535},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := handshakeWireBytes(t, "play.example.com", tt.wire, 2)
			var h Handshake
			require.NoError(t, h.Decode(nil, bytes.NewReader(wire)))
			require.Equal(t, tt.want, h.Port,
				"wire byte pair 0x%04x must decode as the unsigned port %d", tt.wire, tt.want)
		})
	}
}

// TestHandshakePortRoundTrip covers Encode -> Decode for the whole unsigned
// 16-bit domain boundary, including the ports that a signed int16 cannot hold.
func TestHandshakePortRoundTrip(t *testing.T) {
	for _, port := range []int{0, 1, 25565, 32767, 32768, 65535} {
		t.Run(strconv.Itoa(port), func(t *testing.T) {
			sent := &Handshake{
				ProtocolVersion: int(version.Minecraft_1_20_2.Protocol),
				ServerAddress:   "play.example.com",
				Port:            port,
				NextStatus:      2,
			}
			var buf bytes.Buffer
			require.NoError(t, sent.Encode(nil, &buf))

			var got Handshake
			require.NoError(t, got.Decode(nil, bytes.NewReader(buf.Bytes())))
			require.Equal(t, sent.Port, got.Port, "port must survive an Encode/Decode round trip")
			require.Equal(t, sent.ServerAddress, got.ServerAddress)
			require.Equal(t, sent.ProtocolVersion, got.ProtocolVersion)
			require.Equal(t, sent.NextStatus, got.NextStatus)
		})
	}
}

// TestHandshakeEncodeRejectsOutOfRangePort requires an explicit failure instead
// of a silent narrowing for values that simply do not fit the 16-bit wire field.
func TestHandshakeEncodeRejectsOutOfRangePort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"negative", -1},
		{"min signed int16", -32768},
		{"one above uint16 max", 65536},
		{"far above uint16 max", 70000},
		{"max int32", math.MaxInt32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handshake{
				ProtocolVersion: int(version.Minecraft_1_20_2.Protocol),
				ServerAddress:   "play.example.com",
				Port:            tt.port,
				NextStatus:      2,
			}
			var buf bytes.Buffer
			err := h.Encode(nil, &buf)
			require.Error(t, err,
				"port %d is not representable as an unsigned 16-bit wire value and must be rejected, not truncated", tt.port)
			require.Empty(t, buf.Bytes(), "nothing after the port field may be written on a rejected handshake")
		})
	}
}

// TestHandshakeEncodeWireFormatUnchanged pins the exact byte layout for a
// normal (<= 32767) port, so a representation fix cannot silently change the
// protocol encoding.
func TestHandshakeEncodeWireFormatUnchanged(t *testing.T) {
	h := &Handshake{
		ProtocolVersion: 0,
		ServerAddress:   "h",
		Port:            258,
		NextStatus:      2,
	}
	var buf bytes.Buffer
	require.NoError(t, h.Encode(nil, &buf))
	// VarInt protocol 0, string "h" (length 1 + 'h'), big-endian uint16 258,
	// VarInt next status 2.
	require.Equal(t, []byte{0x00, 0x01, 0x68, 0x01, 0x02, 0x02}, buf.Bytes())

	// And the full packet stays byte-identical to the primitive wire builder
	// for every normal port.
	for _, port := range []uint16{0, 1, 25565, 32767} {
		h := &Handshake{
			ProtocolVersion: int(version.Minecraft_1_20_2.Protocol),
			ServerAddress:   "play.example.com",
			Port:            int(port),
			NextStatus:      2,
		}
		var buf bytes.Buffer
		require.NoError(t, h.Encode(nil, &buf))
		require.Equal(t, handshakeWireBytes(t, "play.example.com", port, 2), buf.Bytes(),
			"encoding of port %d must not change", port)
	}
}
