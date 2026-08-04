package connectutil

import (
	"errors"
	"fmt"

	"go.minekube.com/connect"
	"go.minekube.com/connect/bedrockprincipal"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// VerifiedPrincipalProvider exposes the verified Bedrock principal of a tunnel
// connection whose session proposal carried a signed principal v2 envelope.
// Only the Connect SDK verifier can create the principal; Gate never
// constructs one from proposal-supplied profile data.
type VerifiedPrincipalProvider interface {
	VerifiedPrincipal() bedrockprincipal.VerifiedBedrockPrincipal
}

// Frozen minekube.connect.v1alpha1.Session field numbers for the Bedrock
// principal v2 contract. Gate's generated Session descriptor predates these
// fields, so they are read from the unknown-field region by exact number and
// wire type; if a future generated module adds them, they are read from the
// typed fields instead.
const (
	sessionFieldProtocol              = 6
	sessionFieldEndpointID            = 7
	sessionFieldOrganizationID        = 8
	sessionFieldConnectSessionNonce   = 9
	sessionFieldSourceProtocolVersion = 10
	sessionFieldPolicyRevision        = 11
	sessionFieldSignedPrincipalV2     = 12
)

// SessionProtocolBedrock is SESSION_PROTOCOL_BEDROCK in the frozen
// SessionProtocol enum.
const SessionProtocolBedrock int32 = 2

// SessionPrincipalWire carries the frozen Bedrock principal v2 session fields
// of one proposal. Envelope is the opaque compact JWS; every other field is an
// authenticated binding input for the verifier, never identity by itself.
type SessionPrincipalWire struct {
	Protocol              int32
	EndpointID            string
	OrganizationID        string
	ConnectSessionNonce   [16]byte
	SourceProtocolVersion int32
	PolicyRevision        int64
	Envelope              []byte
}

// HasEnvelope reports whether the proposal carried a signed principal envelope.
func (w *SessionPrincipalWire) HasEnvelope() bool {
	return w != nil && len(w.Envelope) > 0
}

// IsBedrock reports whether the proposal declared the Bedrock session protocol.
func (w *SessionPrincipalWire) IsBedrock() bool {
	return w != nil && w.Protocol == SessionProtocolBedrock
}

// ExtractSessionPrincipalWire reads the frozen v2 fields from a session
// proposal. It returns nil when the session carries none of them (a v1
// proposal) and an error when the fields are structurally invalid, in which
// case the proposal must be rejected rather than downgraded.
func ExtractSessionPrincipalWire(s *connect.Session) (*SessionPrincipalWire, error) {
	if s == nil {
		return nil, nil
	}
	m := s.ProtoReflect()
	w := &SessionPrincipalWire{}
	found := false
	var nonce []byte
	haveEnvelope := false

	setString := func(dst *string, v string) { *dst = v; found = true }
	setVarint := func(dst func(int64), v int64) { dst(v); found = true }

	// Typed fields, if the generated descriptor already knows them.
	fields := m.Descriptor().Fields()
	for _, num := range []protowire.Number{
		sessionFieldProtocol, sessionFieldEndpointID, sessionFieldOrganizationID,
		sessionFieldConnectSessionNonce, sessionFieldSourceProtocolVersion,
		sessionFieldPolicyRevision, sessionFieldSignedPrincipalV2,
	} {
		fd := fields.ByNumber(protoreflect.FieldNumber(num))
		if fd == nil || !m.Has(fd) {
			continue
		}
		v := m.Get(fd)
		switch num {
		case sessionFieldProtocol:
			setVarint(func(i int64) { w.Protocol = int32(i) }, int64(v.Enum()))
		case sessionFieldEndpointID:
			setString(&w.EndpointID, v.String())
		case sessionFieldOrganizationID:
			setString(&w.OrganizationID, v.String())
		case sessionFieldConnectSessionNonce:
			nonce = append([]byte(nil), v.Bytes()...)
			found = true
		case sessionFieldSourceProtocolVersion:
			setVarint(func(i int64) { w.SourceProtocolVersion = int32(i) }, v.Int())
		case sessionFieldPolicyRevision:
			setVarint(func(i int64) { w.PolicyRevision = i }, v.Int())
		case sessionFieldSignedPrincipalV2:
			w.Envelope = append([]byte(nil), v.Bytes()...)
			haveEnvelope = len(w.Envelope) > 0
			found = found || haveEnvelope
		}
	}

	// Unknown-field region for descriptors that predate the frozen fields.
	raw := m.GetUnknown()
	for len(raw) > 0 {
		num, typ, n := protowire.ConsumeTag(raw)
		if n < 0 {
			return nil, fmt.Errorf("invalid session field encoding: %w", protowire.ParseError(n))
		}
		raw = raw[n:]
		isPrincipalField := num >= sessionFieldProtocol && num <= sessionFieldSignedPrincipalV2
		wantBytes := num == sessionFieldEndpointID || num == sessionFieldOrganizationID ||
			num == sessionFieldConnectSessionNonce || num == sessionFieldSignedPrincipalV2
		if !isPrincipalField || (wantBytes && typ != protowire.BytesType) || (!wantBytes && typ != protowire.VarintType) {
			if isPrincipalField {
				return nil, fmt.Errorf("session field %d has unexpected wire type %d", num, typ)
			}
			n = protowire.ConsumeFieldValue(num, typ, raw)
			if n < 0 {
				return nil, fmt.Errorf("invalid session field encoding: %w", protowire.ParseError(n))
			}
			raw = raw[n:]
			continue
		}
		if wantBytes {
			v, n := protowire.ConsumeBytes(raw)
			if n < 0 {
				return nil, fmt.Errorf("invalid session field encoding: %w", protowire.ParseError(n))
			}
			raw = raw[n:]
			switch num {
			case sessionFieldEndpointID:
				setString(&w.EndpointID, string(v))
			case sessionFieldOrganizationID:
				setString(&w.OrganizationID, string(v))
			case sessionFieldConnectSessionNonce:
				nonce = append([]byte(nil), v...)
				found = true
			case sessionFieldSignedPrincipalV2:
				if haveEnvelope {
					return nil, errors.New("session carries more than one signed principal envelope")
				}
				if len(v) == 0 || len(v) > bedrockprincipal.MaxEnvelopeBytes {
					return nil, errors.New("signed principal envelope has invalid size")
				}
				w.Envelope = append([]byte(nil), v...)
				haveEnvelope = true
				found = true
			}
			continue
		}
		v, n := protowire.ConsumeVarint(raw)
		if n < 0 {
			return nil, fmt.Errorf("invalid session field encoding: %w", protowire.ParseError(n))
		}
		raw = raw[n:]
		switch num {
		case sessionFieldProtocol:
			setVarint(func(i int64) { w.Protocol = int32(i) }, int64(v))
		case sessionFieldSourceProtocolVersion:
			setVarint(func(i int64) { w.SourceProtocolVersion = int32(i) }, int64(v))
		case sessionFieldPolicyRevision:
			setVarint(func(i int64) { w.PolicyRevision = i }, int64(v))
		}
	}

	if !found {
		return nil, nil
	}
	if haveEnvelope {
		if len(nonce) != len(w.ConnectSessionNonce) {
			return nil, errors.New("signed principal session nonce has invalid size")
		}
		copy(w.ConnectSessionNonce[:], nonce)
	}
	return w, nil
}
