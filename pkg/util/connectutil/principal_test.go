package connectutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/connect"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// appendV2Fields encodes the frozen Session v2 fields into the wire form an
// up-to-date Connect edge sends; Gate's generated descriptor keeps them in
// the unknown-field region.
type v2Fields struct {
	protocol              *int64
	endpointID            *string
	organizationID        *string
	nonce                 []byte
	sourceProtocolVersion *int64
	policyRevision        *int64
	envelopes             [][]byte
	envelopeAsVarint      bool
}

func sessionWithV2(t *testing.T, f v2Fields) *connect.Session {
	t.Helper()
	raw, err := proto.Marshal(&connect.Session{Id: "sess-1"})
	require.NoError(t, err)
	if f.protocol != nil {
		raw = protowire.AppendTag(raw, sessionFieldProtocol, protowire.VarintType)
		raw = protowire.AppendVarint(raw, uint64(*f.protocol))
	}
	if f.endpointID != nil {
		raw = protowire.AppendTag(raw, sessionFieldEndpointID, protowire.BytesType)
		raw = protowire.AppendString(raw, *f.endpointID)
	}
	if f.organizationID != nil {
		raw = protowire.AppendTag(raw, sessionFieldOrganizationID, protowire.BytesType)
		raw = protowire.AppendString(raw, *f.organizationID)
	}
	if f.nonce != nil {
		raw = protowire.AppendTag(raw, sessionFieldConnectSessionNonce, protowire.BytesType)
		raw = protowire.AppendBytes(raw, f.nonce)
	}
	if f.sourceProtocolVersion != nil {
		raw = protowire.AppendTag(raw, sessionFieldSourceProtocolVersion, protowire.VarintType)
		raw = protowire.AppendVarint(raw, uint64(*f.sourceProtocolVersion))
	}
	if f.policyRevision != nil {
		raw = protowire.AppendTag(raw, sessionFieldPolicyRevision, protowire.VarintType)
		raw = protowire.AppendVarint(raw, uint64(*f.policyRevision))
	}
	for _, envelope := range f.envelopes {
		if f.envelopeAsVarint {
			raw = protowire.AppendTag(raw, sessionFieldSignedPrincipalV2, protowire.VarintType)
			raw = protowire.AppendVarint(raw, 1)
			continue
		}
		raw = protowire.AppendTag(raw, sessionFieldSignedPrincipalV2, protowire.BytesType)
		raw = protowire.AppendBytes(raw, envelope)
	}
	s := new(connect.Session)
	require.NoError(t, proto.Unmarshal(raw, s))
	return s
}

func ptrI64(v int64) *int64   { return &v }
func ptrStr(v string) *string { return &v }
func nonce16() []byte         { return []byte("0123456789abcdef") }
func fullV2(env []byte) v2Fields {
	return v2Fields{
		protocol:              ptrI64(int64(SessionProtocolBedrock)),
		endpointID:            ptrStr("endpoint-1"),
		organizationID:        ptrStr("org-1"),
		nonce:                 nonce16(),
		sourceProtocolVersion: ptrI64(3),
		policyRevision:        ptrI64(7),
		envelopes:             [][]byte{env},
	}
}

func TestExtractSessionPrincipalWireAbsent(t *testing.T) {
	wire, err := ExtractSessionPrincipalWire(&connect.Session{Id: "sess-1"})
	require.NoError(t, err)
	require.Nil(t, wire)
	require.False(t, wire.HasEnvelope())
	require.False(t, wire.IsBedrock())

	wire, err = ExtractSessionPrincipalWire(nil)
	require.NoError(t, err)
	require.Nil(t, wire)
}

func TestExtractSessionPrincipalWireFull(t *testing.T) {
	envelope := []byte("header.payload.signature")
	wire, err := ExtractSessionPrincipalWire(sessionWithV2(t, fullV2(envelope)))
	require.NoError(t, err)
	require.NotNil(t, wire)
	require.True(t, wire.HasEnvelope())
	require.True(t, wire.IsBedrock())
	require.Equal(t, "endpoint-1", wire.EndpointID)
	require.Equal(t, "org-1", wire.OrganizationID)
	require.Equal(t, [16]byte([]byte("0123456789abcdef")), wire.ConnectSessionNonce)
	require.Equal(t, int32(3), wire.SourceProtocolVersion)
	require.Equal(t, int64(7), wire.PolicyRevision)
	require.Equal(t, envelope, wire.Envelope)
}

func TestExtractSessionPrincipalWireProtocolOnly(t *testing.T) {
	wire, err := ExtractSessionPrincipalWire(sessionWithV2(t, v2Fields{
		protocol: ptrI64(int64(SessionProtocolBedrock)),
	}))
	require.NoError(t, err)
	require.NotNil(t, wire)
	require.True(t, wire.IsBedrock())
	require.False(t, wire.HasEnvelope())
}

func TestExtractSessionPrincipalWireRejectsDuplicateEnvelope(t *testing.T) {
	f := fullV2([]byte("first"))
	f.envelopes = append(f.envelopes, []byte("second"))
	_, err := ExtractSessionPrincipalWire(sessionWithV2(t, f))
	require.Error(t, err)
}

func TestExtractSessionPrincipalWireRejectsEmptyEnvelopeField(t *testing.T) {
	f := fullV2(nil)
	_, err := ExtractSessionPrincipalWire(sessionWithV2(t, f))
	require.Error(t, err)
}

func TestExtractSessionPrincipalWireRejectsBadNonce(t *testing.T) {
	f := fullV2([]byte("envelope"))
	f.nonce = []byte("short")
	_, err := ExtractSessionPrincipalWire(sessionWithV2(t, f))
	require.Error(t, err)

	f = fullV2([]byte("envelope"))
	f.nonce = nil
	_, err = ExtractSessionPrincipalWire(sessionWithV2(t, f))
	require.Error(t, err)
}

func TestExtractSessionPrincipalWireRejectsWrongWireType(t *testing.T) {
	f := fullV2([]byte("envelope"))
	f.envelopeAsVarint = true
	_, err := ExtractSessionPrincipalWire(sessionWithV2(t, f))
	require.Error(t, err)
}
