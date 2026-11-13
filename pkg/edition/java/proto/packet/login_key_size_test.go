package packet

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// TestEncryptionPacketsCarryLargerLoginKeys pins that the two login handshake
// encryption packets round-trip the sizes a larger login key produces: the
// server's DER encoded public key in the EncryptionRequest and the RSA
// ciphertexts of the shared secret and the verify token in the
// EncryptionResponse. Their decoders used to cap the encoded byte arrays at
// 128/256 bytes, i.e. at exactly one 1024 bit RSA block, so a bigger
// auth.privateKeyBits could not complete a login at all.
func TestEncryptionPacketsCarryLargerLoginKeys(t *testing.T) {
	protocols := []*proto.Version{
		version.Minecraft_1_12_2, // legacy short-prefixed byte arrays
		version.Minecraft_1_19_1, // verify token without salt
		version.Minecraft_1_20,   // modern login
		version.MaximumVersion,
	}

	for _, bits := range []int{1024, 2048, 4096} {
		t.Run(fmt.Sprintf("%d_bit_key", bits), func(t *testing.T) {
			key, err := rsa.GenerateKey(rand.Reader, bits)
			require.NoError(t, err)
			pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
			require.NoError(t, err)

			encSecret, err := rsa.EncryptPKCS1v15(rand.Reader, &key.PublicKey, make([]byte, 16))
			require.NoError(t, err)
			encToken, err := rsa.EncryptPKCS1v15(rand.Reader, &key.PublicKey, make([]byte, 4))
			require.NoError(t, err)
			// The test premise: ciphertext length follows the login key size.
			require.Len(t, encSecret, bits/8)
			require.Len(t, encToken, bits/8)

			for _, v := range protocols {
				c := &proto.PacketContext{Direction: proto.ClientBound, Protocol: v.Protocol}

				req := &EncryptionRequest{
					ServerID:    "serverid",
					PublicKey:   pubDER,
					VerifyToken: []byte{1, 2, 3, 4},
				}
				buf := new(bytes.Buffer)
				require.NoError(t, req.Encode(c, buf), "request encode %s", v)
				gotReq := new(EncryptionRequest)
				require.NoError(t, gotReq.Decode(c, buf), "request decode %s", v)
				require.Equal(t, req.PublicKey, gotReq.PublicKey, "request public key %s", v)
				require.Equal(t, req.VerifyToken, gotReq.VerifyToken, "request verify token %s", v)

				c.Direction = proto.ServerBound
				resp := &EncryptionResponse{SharedSecret: encSecret, VerifyToken: encToken}
				buf = new(bytes.Buffer)
				require.NoError(t, resp.Encode(c, buf), "response encode %s", v)
				gotResp := new(EncryptionResponse)
				require.NoError(t, gotResp.Decode(c, buf), "response decode %s", v)
				require.Equal(t, encSecret, gotResp.SharedSecret, "response shared secret %s", v)
				require.Equal(t, encToken, gotResp.VerifyToken, "response verify token %s", v)
			}
		})
	}
}

// maxSupportedLoginKeyBits mirrors config.MaxPrivateKeyBits: the largest login
// key Gate can be configured to generate has to fit the packet bounds.
const maxSupportedLoginKeyBits = 8192

// TestLoginEncryptionPacketBoundsCoverTheLargestLoginKey keeps the packet
// bounds and the configurable key size range from drifting apart, without
// paying for an 8192 bit key generation.
func TestLoginEncryptionPacketBoundsCoverTheLargestLoginKey(t *testing.T) {
	require.GreaterOrEqual(t, maxLoginEncryptionBytes*8, maxSupportedLoginKeyBits,
		"the login encryption packet bounds must cover the largest configurable login key")

	c := &proto.PacketContext{Direction: proto.ClientBound, Protocol: version.Minecraft_1_20.Protocol}
	payload := make([]byte, maxLoginEncryptionBytes)

	req := &EncryptionRequest{ServerID: "serverid", PublicKey: payload, VerifyToken: payload[:4]}
	buf := new(bytes.Buffer)
	require.NoError(t, req.Encode(c, buf))
	gotReq := new(EncryptionRequest)
	require.NoError(t, gotReq.Decode(c, buf))
	require.Equal(t, payload, gotReq.PublicKey)

	c.Direction = proto.ServerBound
	resp := &EncryptionResponse{SharedSecret: payload, VerifyToken: payload}
	buf = new(bytes.Buffer)
	require.NoError(t, resp.Encode(c, buf))
	gotResp := new(EncryptionResponse)
	require.NoError(t, gotResp.Decode(c, buf))
	require.Equal(t, payload, gotResp.SharedSecret)
	require.Equal(t, payload, gotResp.VerifyToken)
}
