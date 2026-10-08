package config

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.minekube.com/connect"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/connectutil"
	"go.minekube.com/gate/pkg/util/connectutil/config/internal/bedrockidentity"
	"go.minekube.com/gate/pkg/util/uuid"
)

const identityMetadataURL = "https://watch-connect.minekube.net/.well-known/minekube-connect/bedrock-identity-keys.json"

var (
	errIdentityInvalid     = errors.New("invalid signed bedrock identity")
	errIdentityUnavailable = errors.New("bedrock identity verifier keys unavailable")
)

// identityVerifier consumes the production Connect v1 contract. Trust is
// automatic only for Minekube's authenticated Watch service; a custom Watch
// service must never gain Minekube's identity authority from configuration.
// Explicit v2 require-mode keeps its existing verification contract.
type identityVerifier struct {
	endpoint string
	keys     *identityKeys
	replay   *bedrockidentity.ReplayCache
	now      func() time.Time
}

func newIdentityVerifier(c Config) *identityVerifier {
	u, err := url.Parse(c.WatchServiceAddr)
	if c.EnforcePassthrough || err != nil || u.Scheme != "wss" || u.Host != "watch-connect.minekube.net" ||
		u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" ||
		strings.EqualFold(strings.TrimSpace(c.BedrockPrincipal.Mode), bedrockPrincipalModeRequire) {
		return nil
	}
	return &identityVerifier{
		keys:   newIdentityKeys(),
		replay: bedrockidentity.NewReplayCache(),
		now:    time.Now,
	}
}

// gameProfile returns only a verified, session-bound profile, or nil for a
// Java/legacy proposal without reserved metadata. All errors are bounded and
// contain no envelope, profile, key, address or remote HTTP response material.
func (v *identityVerifier) gameProfile(ctx context.Context, s *connect.Session, wire *connectutil.SessionPrincipalWire) (*profile.GameProfile, error) {
	var envelope, scope string
	var envelopes, scopes int
	for _, p := range s.GetPlayer().GetProfile().GetProperties() {
		switch p.GetName() {
		case bedrockidentity.PropertyName:
			envelopes++
			envelope = p.GetValue()
		case bedrockidentity.ScopePropertyName:
			scopes++
			scope = p.GetValue()
		}
	}
	if envelopes == 0 && scopes == 0 && !wire.IsBedrock() {
		return nil, nil
	}
	if v == nil || envelopes != 1 || scopes != 1 || !wire.IsBedrock() || wire.HasEnvelope() ||
		s.GetAuth().GetPassthrough() || len(scope) > 4096 {
		return nil, errIdentityInvalid
	}
	binding, err := bedrockidentity.ParseScopeProperty(scope)
	if err != nil {
		return nil, errIdentityInvalid
	}
	proposed := s.GetPlayer().GetProfile()
	id, err := uuid.Parse(proposed.GetId())
	if err != nil || id == uuid.Nil || proposed.GetName() == "" {
		return nil, errIdentityInvalid
	}
	opts := bedrockidentity.VerifyOptions{
		ExpectedIssuer: "minekube-connect", EndpointID: binding.EndpointID,
		EndpointName: v.endpoint, OrgID: binding.EndpointOrgID, SessionID: s.GetId(),
		Protocol: "bedrock", Now: v.now, Replay: v.replay,
	}
	keys, err := v.keys.get(ctx, false)
	if err != nil {
		return nil, errIdentityUnavailable
	}
	claims, err := verifyIdentityKeys(envelope, keys, opts)
	if err != nil {
		// A rotated signing key can appear before our cached metadata expires.
		// Refresh at most once per backoff interval, then retry verification.
		keys, fetchErr := v.keys.get(ctx, true)
		if fetchErr == nil {
			claims, err = verifyIdentityKeys(envelope, keys, opts)
		}
	}
	if err != nil {
		return nil, errIdentityInvalid
	}
	expectedID := claims.BedrockDerivedUUID
	name := proposed.GetName()
	if claims.PrincipalType == bedrockidentity.PrincipalBedrockLinkedJava {
		expectedID = claims.LinkedJavaUUID
		if !strings.EqualFold(name, claims.LinkedJavaName) {
			return nil, errIdentityInvalid
		}
		name = claims.LinkedJavaName
	}
	verifiedID, err := uuid.Parse(expectedID)
	if err != nil || verifiedID != id {
		return nil, errIdentityInvalid
	}
	gp, err := convertProposedGameProfile(proposed)
	if err != nil {
		return nil, errIdentityInvalid
	}
	gp.ID, gp.Name = verifiedID, name
	// Reserved envelopes belong only to this authenticated tunnel and must
	// never be relayed to backends as ordinary profile properties.
	props := gp.Properties[:0]
	for _, p := range gp.Properties {
		if p.Name != bedrockidentity.PropertyName && p.Name != bedrockidentity.ScopePropertyName {
			props = append(props, p)
		}
	}
	gp.Properties = props
	return gp, nil
}

func verifyIdentityKeys(envelope string, keys []ed25519.PublicKey, opts bedrockidentity.VerifyOptions) (bedrockidentity.Claims, error) {
	for _, key := range keys {
		opts.Public = key
		claims, err := bedrockidentity.Verify(envelope, opts)
		if err == nil {
			return claims, nil
		}
	}
	return bedrockidentity.Claims{}, errIdentityInvalid
}

type identityKeys struct {
	mu                               sync.Mutex
	client                           *http.Client
	now                              func() time.Time
	keys                             []ed25519.PublicKey
	expires, staleUntil, nextRefresh time.Time
}

func newIdentityKeys() *identityKeys {
	return &identityKeys{
		client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		now:    time.Now,
	}
}

func (k *identityKeys) get(ctx context.Context, refresh bool) ([]ed25519.PublicKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.now()
	if (!refresh && now.Before(k.expires)) || now.Before(k.nextRefresh) {
		if now.Before(k.staleUntil) {
			return k.keys, nil
		}
		return nil, errIdentityUnavailable
	}
	keys, ttl, err := k.fetch(ctx)
	now = k.now()
	k.nextRefresh = now.Add(5 * time.Second)
	if err != nil {
		if now.Before(k.staleUntil) {
			return k.keys, nil
		}
		return nil, errIdentityUnavailable
	}
	k.keys = keys
	k.expires = now.Add(ttl)
	k.staleUntil = k.expires.Add(10 * time.Minute)
	return k.keys, nil
}

func (k *identityKeys) fetch(ctx context.Context) ([]ed25519.PublicKey, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, identityMetadataURL, nil)
	if err != nil {
		return nil, 0, errIdentityUnavailable
	}
	res, err := k.client.Do(req)
	if err != nil {
		return nil, 0, errIdentityUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, 0, errIdentityUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 64*1024+1))
	if err != nil || len(body) > 64*1024 {
		return nil, 0, errIdentityUnavailable
	}
	var metadata struct {
		Issuer    string   `json:"issuer"`
		Algorithm string   `json:"algorithm"`
		Current   string   `json:"current_public_key"`
		Previous  []string `json:"previous_public_keys"`
		MaxAge    int64    `json:"cache_max_age_seconds"`
	}
	if json.Unmarshal(body, &metadata) != nil || metadata.Issuer != "minekube-connect" || metadata.Algorithm != "Ed25519" || len(metadata.Previous) > 16 {
		return nil, 0, errIdentityUnavailable
	}
	var keys []ed25519.PublicKey
	for _, encoded := range append([]string{metadata.Current}, metadata.Previous...) {
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) != ed25519.PublicKeySize {
			return nil, 0, errIdentityUnavailable
		}
		keys = append(keys, ed25519.PublicKey(key))
	}
	ttl := 5 * time.Minute
	if metadata.MaxAge > 0 && metadata.MaxAge < 300 {
		ttl = time.Duration(metadata.MaxAge) * time.Second
	}
	return keys, ttl, nil
}
