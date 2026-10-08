// The v1 wire format and signature canonicalization follow Minekube Moxy
// connect/bedrockauth/envelope.go. Interoperability is pinned by its signed
// fixtures in testdata/v1. Errors here are diagnostic-only: callers must map
// them to bounded categories before logging or rejecting a proposal.
package bedrockidentity

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	EnvelopeVersion   = 1
	Capability        = "bedrock-identity-v1"
	PropertyName      = "minekube:bedrock_identity"
	ScopePropertyName = "minekube:bedrock_identity_scope"

	MaxEnvelopeTTL   = 5 * time.Minute
	MaxFutureSkew    = time.Minute
	MaxReplayEntries = 4096
	MaxEnvelopeBytes = 16 * 1024
)

type PrincipalType string

const (
	PrincipalBedrockXUID       PrincipalType = "bedrock_xuid"
	PrincipalBedrockLinkedJava PrincipalType = "bedrock_linked_java"
)

type Claims struct {
	EndpointID         string
	EndpointName       string
	OrgID              string
	SessionID          string
	Protocol           string
	BedrockAuthPolicy  Policy
	PrincipalType      PrincipalType
	BedrockXUID        string
	BedrockUsername    string
	BedrockDerivedUUID string
	LinkedJavaUUID     string
	LinkedJavaName     string
}

type ScopeProperty struct {
	EndpointID    string `json:"endpoint_id"`
	EndpointOrgID string `json:"endpoint_org_id"`
}

func NewScopeProperty(endpointID, endpointOrgID string) (string, error) {
	value, err := json.Marshal(ScopeProperty{
		EndpointID:    endpointID,
		EndpointOrgID: endpointOrgID,
	})
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func ParseScopeProperty(value string) (ScopeProperty, error) {
	if len(value) > 4096 {
		return ScopeProperty{}, errors.New("invalid identity scope size")
	}
	if err := rejectDuplicateJSONFields([]byte(value)); err != nil {
		return ScopeProperty{}, err
	}
	var scope struct {
		EndpointID string  `json:"endpoint_id"`
		OrgID      *string `json:"endpoint_org_id"`
	}
	d := json.NewDecoder(strings.NewReader(value))
	d.DisallowUnknownFields()
	if err := d.Decode(&scope); err != nil {
		return ScopeProperty{}, err
	}
	if scope.EndpointID == "" || scope.OrgID == nil {
		return ScopeProperty{}, errors.New("incomplete identity scope")
	}
	return ScopeProperty{EndpointID: scope.EndpointID, EndpointOrgID: *scope.OrgID}, nil
}

type VerifyOptions struct {
	Public         ed25519.PublicKey
	ExpectedIssuer string
	Now            func() time.Time
	EndpointID     string
	EndpointName   string
	OrgID          string
	SessionID      string
	Protocol       string
	Replay         *ReplayCache
}

type ReplayCache struct {
	mu   sync.Mutex
	seen map[string]int64
}

func NewReplayCache() *ReplayCache {
	return &ReplayCache{seen: make(map[string]int64)}
}

func (c *ReplayCache) accept(key string, nowUnixMS, expiresAtUnixMS int64) bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for nonce, expiresAt := range c.seen {
		if expiresAt <= nowUnixMS {
			delete(c.seen, nonce)
		}
	}
	if _, ok := c.seen[key]; ok {
		return false
	}
	if len(c.seen) >= MaxReplayEntries {
		return false
	}
	c.seen[key] = expiresAtUnixMS
	return true
}

type envelope struct {
	Version   int               `json:"version"`
	Issuer    string            `json:"issuer"`
	Endpoint  envelopeEndpoint  `json:"endpoint"`
	Session   envelopeSession   `json:"session"`
	Policy    envelopePolicy    `json:"policy"`
	Principal envelopePrincipal `json:"principal"`
	Signature string            `json:"signature,omitempty"`
}

type envelopeEndpoint struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	OrgID string `json:"org_id"`
}

type envelopeSession struct {
	ID              string `json:"id"`
	Protocol        string `json:"protocol"`
	IssuedAtUnixMS  int64  `json:"issued_at_unix_ms"`
	ExpiresAtUnixMS int64  `json:"expires_at_unix_ms"`
	Nonce           string `json:"nonce"`
}

type envelopePolicy struct {
	BedrockAuthMode Policy `json:"bedrock_auth_mode"`
}

type envelopePrincipal struct {
	Type               PrincipalType `json:"type"`
	BedrockXUID        string        `json:"bedrock_xuid,omitempty"`
	BedrockUsername    string        `json:"bedrock_username,omitempty"`
	BedrockDerivedUUID string        `json:"bedrock_derived_uuid,omitempty"`
	LinkedJavaUUID     string        `json:"linked_java_uuid,omitempty"`
	LinkedJavaName     string        `json:"linked_java_name,omitempty"`
}

func Verify(signed string, opts VerifyOptions) (Claims, error) {
	if len(opts.Public) != ed25519.PublicKeySize {
		return Claims{}, errors.New("public key must be ed25519 public key")
	}
	env, err := decodeEnvelope(signed)
	if err != nil {
		return Claims{}, fmt.Errorf("decode envelope: %w", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(env.Signature)
	if err != nil {
		return Claims{}, fmt.Errorf("decode signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return Claims{}, errors.New("invalid envelope signature size")
	}
	payload, err := signingPayload(env)
	if err != nil {
		return Claims{}, err
	}
	if !ed25519.Verify(opts.Public, payload, signature) {
		return Claims{}, errors.New("invalid envelope signature")
	}
	if err := validateEnvelope(env); err != nil {
		return Claims{}, err
	}
	if opts.ExpectedIssuer == "" {
		return Claims{}, errors.New("expected identity envelope issuer is required")
	}
	if env.Issuer != opts.ExpectedIssuer {
		return Claims{}, errors.New("identity envelope issuer mismatch")
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	nowUnixMS := now().UnixMilli()
	if env.Session.IssuedAtUnixMS > nowUnixMS+MaxFutureSkew.Milliseconds() {
		return Claims{}, errors.New("identity envelope issued too far in the future")
	}
	if nowUnixMS >= env.Session.ExpiresAtUnixMS {
		return Claims{}, errors.New("identity envelope expired")
	}
	if env.Endpoint.ID != opts.EndpointID ||
		env.Endpoint.Name != opts.EndpointName ||
		env.Endpoint.OrgID != opts.OrgID ||
		env.Session.ID != opts.SessionID ||
		env.Session.Protocol != opts.Protocol {
		return Claims{}, errors.New("identity envelope scope mismatch")
	}
	replayKey := env.Endpoint.ID + "\x00" + env.Session.ID + "\x00" + env.Session.Nonce
	if !opts.Replay.accept(replayKey, nowUnixMS, env.Session.ExpiresAtUnixMS) {
		return Claims{}, errors.New("identity envelope replayed")
	}
	return claimsFromEnvelope(env), nil
}

func decodeEnvelope(signed string) (envelope, error) {
	if len(signed) == 0 || len(signed) > MaxEnvelopeBytes {
		return envelope{}, errors.New("invalid identity envelope size")
	}
	if err := rejectDuplicateJSONFields([]byte(signed)); err != nil {
		return envelope{}, err
	}
	if err := requireEnvelopeOrgID([]byte(signed)); err != nil {
		return envelope{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(signed))
	decoder.DisallowUnknownFields()
	var env envelope
	if err := decoder.Decode(&env); err != nil {
		return envelope{}, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return envelope{}, err
	}
	return env, nil
}

// requireEnvelopeOrgID keeps the organization dimension present in the v1
// wire contract while allowing its signed value to be empty. A missing or null
// field must never collapse into an endpoint-scoped value through Go's string
// zero value.
func requireEnvelopeOrgID(data []byte) error {
	var document struct {
		Endpoint map[string]json.RawMessage `json:"endpoint"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	raw, ok := document.Endpoint["org_id"]
	if !ok || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '"' {
		return errors.New("identity envelope endpoint organization scope is required")
	}
	var orgID string
	if err := json.Unmarshal(raw, &orgID); err != nil {
		return fmt.Errorf("decode identity envelope endpoint organization scope: %w", err)
	}
	return nil
}

func rejectDuplicateJSONFields(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}

func requireJSONEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func signingPayload(env envelope) ([]byte, error) {
	env.Signature = ""
	return json.Marshal(env)
}

func validateEnvelope(env envelope) error {
	if env.Version != EnvelopeVersion {
		return fmt.Errorf("unsupported identity envelope version %d", env.Version)
	}
	if env.Issuer == "" {
		return errors.New("identity envelope issuer is required")
	}
	// OrgID is a signed, exact-match scope dimension, but an endpoint without a
	// dashboard organization is intentionally endpoint-scoped. Endpoint ID and
	// name remain mandatory so an empty org can never become an unbound scope.
	if env.Endpoint.ID == "" || env.Endpoint.Name == "" {
		return errors.New("identity envelope endpoint scope is incomplete")
	}
	if env.Session.ID == "" || env.Session.Protocol != "bedrock" || env.Session.Nonce == "" {
		return errors.New("identity envelope session scope is incomplete or invalid")
	}
	if env.Session.IssuedAtUnixMS <= 0 || env.Session.ExpiresAtUnixMS <= env.Session.IssuedAtUnixMS {
		return errors.New("identity envelope expiry must be after a valid issue time")
	}
	if lifetimeMS := env.Session.ExpiresAtUnixMS - env.Session.IssuedAtUnixMS; lifetimeMS > MaxEnvelopeTTL.Milliseconds() {
		return fmt.Errorf("identity envelope lifetime exceeds %s", MaxEnvelopeTTL)
	}
	nonce, err := base64.RawURLEncoding.DecodeString(env.Session.Nonce)
	if err != nil || len(nonce) != 16 {
		return errors.New("identity envelope nonce must be 16 base64url bytes")
	}
	if !env.Policy.BedrockAuthMode.Valid() {
		return errors.New("identity envelope policy is invalid")
	}
	if _, err := ParseCanonicalXUID(env.Principal.BedrockXUID); err != nil {
		return err
	}
	if strings.TrimSpace(env.Principal.BedrockUsername) == "" {
		return errors.New("bedrock principal requires username display metadata")
	}
	derivedUUID, err := DerivedUUIDForXUID(env.Principal.BedrockXUID)
	if err != nil || env.Principal.BedrockDerivedUUID != derivedUUID {
		return errors.New("bedrock principal derived uuid does not match xuid")
	}
	switch env.Principal.Type {
	case PrincipalBedrockXUID:
		if env.Policy.BedrockAuthMode != PolicyTrustedBedrockXUID {
			return errors.New("bedrock_xuid principal requires trusted_bedrock_xuid policy")
		}
		if env.Principal.LinkedJavaUUID != "" || env.Principal.LinkedJavaName != "" {
			return errors.New("bedrock_xuid principal cannot contain linked Java identity")
		}
	case PrincipalBedrockLinkedJava:
		linkedID, err := uuid.Parse(env.Principal.LinkedJavaUUID)
		if err != nil || linkedID == uuid.Nil || strings.TrimSpace(env.Principal.LinkedJavaName) == "" {
			return errors.New("bedrock_linked_java principal requires valid linked Java identity")
		}
	default:
		return fmt.Errorf("unsupported identity envelope principal type %q", env.Principal.Type)
	}
	return nil
}

func claimsFromEnvelope(env envelope) Claims {
	return Claims{
		EndpointID:         env.Endpoint.ID,
		EndpointName:       env.Endpoint.Name,
		OrgID:              env.Endpoint.OrgID,
		SessionID:          env.Session.ID,
		Protocol:           env.Session.Protocol,
		BedrockAuthPolicy:  env.Policy.BedrockAuthMode,
		PrincipalType:      env.Principal.Type,
		BedrockXUID:        env.Principal.BedrockXUID,
		BedrockUsername:    env.Principal.BedrockUsername,
		BedrockDerivedUUID: env.Principal.BedrockDerivedUUID,
		LinkedJavaUUID:     env.Principal.LinkedJavaUUID,
		LinkedJavaName:     env.Principal.LinkedJavaName,
	}
}
