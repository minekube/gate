package config

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"go.minekube.com/connect"
	"go.minekube.com/connect/ws"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/connectutil"
	"go.minekube.com/gate/pkg/util/connectutil/config/internal/bedrockidentity"
)

type identityTransport func(*http.Request) (*http.Response, error)

func (f identityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixtureIdentity(t *testing.T, orgless bool) (*identityVerifier, *connect.Session) {
	t.Helper()
	v := newIdentityVerifier(DefaultConfig)
	require.NotNil(t, v)
	v.endpoint = "fixture-endpoint"
	v.now = func() time.Time { return time.UnixMilli(1783260000000) }
	key, err := base64.StdEncoding.DecodeString("EvLQyZOnKxAUFaKqU2AhyOLXzPL6cHBNZjqwiLPxQy4=")
	require.NoError(t, err)
	v.keys.keys = []ed25519.PublicKey{key}
	v.keys.expires = time.Now().Add(time.Hour)
	v.keys.staleUntil = time.Now().Add(time.Hour)
	v.keys.nextRefresh = time.Now().Add(time.Hour) // malformed input must not hit the internet
	file, org := "v1-bedrock-xuid-valid.json", "org-id"
	if orgless {
		file, org = "v1-bedrock-xuid-orgless-valid.json", ""
	}
	envelope, err := os.ReadFile(filepath.Join("internal/bedrockidentity/testdata/v1", file))
	require.NoError(t, err)
	scope, err := bedrockidentity.NewScopeProperty("endpoint-id", org)
	require.NoError(t, err)
	s := &connect.Session{
		Id: "session-id", TunnelServiceAddr: "wss://tunnel.invalid",
		Player: &connect.Player{Addr: "192.0.2.1:19132", Profile: &connect.GameProfile{
			Id: "cafc7598-0ef3-527f-8f28-60af2d9ca6bc", Name: "_BedrockFox",
			Properties: []*connect.GameProfileProperty{
				{Name: bedrockidentity.PropertyName, Value: string(envelope)},
				{Name: bedrockidentity.ScopePropertyName, Value: scope},
				{Name: "textures", Value: "texture-test"},
			},
		}},
	}
	s.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 6, protowire.VarintType), 2))
	return v, s
}

func TestIdentityDefaultsTrustOnlyOfficialWatch(t *testing.T) {
	require.NotNil(t, newIdentityVerifier(DefaultConfig))
	for _, addr := range []string{
		"ws://watch-connect.minekube.net", "wss://watch-connect.minekube.net.attacker.test",
		"wss://attacker.test", "wss://attacker@watch-connect.minekube.net",
		"wss://watch-connect.minekube.net?issuer=other", "wss://watch-connect.minekube.net/other",
	} {
		c := DefaultConfig
		c.WatchServiceAddr = addr
		require.Nil(t, newIdentityVerifier(c))
	}
	c := DefaultConfig
	c.BedrockPrincipal.Mode = "require"
	require.Nil(t, newIdentityVerifier(c), "explicit v2 enforcement must not be downgraded")
	c = DefaultConfig
	c.EnforcePassthrough = true
	require.Nil(t, newIdentityVerifier(c), "pass-through-only connectors cannot accept translated identities")
}

func TestIdentityVerifiesOrgAndEndpointScopedProfiles(t *testing.T) {
	for _, orgless := range []bool{false, true} {
		v, s := fixtureIdentity(t, orgless)
		wire, err := connectutil.ExtractSessionPrincipalWire(s)
		require.NoError(t, err)
		gp, err := v.gameProfile(context.Background(), s, wire)
		require.NoError(t, err)
		require.Equal(t, s.GetPlayer().GetProfile().GetId(), gp.ID.String())
		require.Equal(t, "_BedrockFox", gp.Name)
		require.Equal(t, []profile.Property{{Name: "textures", Value: "texture-test"}}, gp.Properties)
		conn := wrapTunnelSession(nil, s, gp, nil)
		provider, ok := netmc.Assert[proxy.GameProfileProvider](conn)
		require.True(t, ok)
		require.Equal(t, gp, provider.GameProfile())
		_, err = v.gameProfile(context.Background(), s, wire)
		require.ErrorIs(t, err, errIdentityInvalid)
	}
}

func TestIdentityRejectsTamperingBeforeTunnel(t *testing.T) {
	cases := map[string]func(*identityVerifier, *connect.Session){
		"profile": func(_ *identityVerifier, s *connect.Session) {
			s.Player.Profile.Id = "00000000-0000-0000-0000-000000000001"
		},
		"endpoint": func(v *identityVerifier, _ *connect.Session) { v.endpoint = "another-endpoint" },
		"session":  func(_ *identityVerifier, s *connect.Session) { s.Id = "another-session" },
		"signature": func(_ *identityVerifier, s *connect.Session) {
			s.Player.Profile.Properties[0].Value = strings.Replace(s.Player.Profile.Properties[0].Value, "BedrockFox", "OtherPlayer", 1)
		},
		"duplicate": func(_ *identityVerifier, s *connect.Session) {
			s.Player.Profile.Properties = append(s.Player.Profile.Properties, s.Player.Profile.Properties[0])
		},
		"scope": func(_ *identityVerifier, s *connect.Session) {
			s.Player.Profile.Properties[1].Value = `{"endpoint_id":"endpoint-id","endpoint_org_id":"other-org"}`
		},
		"null org": func(_ *identityVerifier, s *connect.Session) {
			s.Player.Profile.Properties[1].Value = `{"endpoint_id":"endpoint-id","endpoint_org_id":null}`
		},
		"duplicate org": func(_ *identityVerifier, s *connect.Session) {
			s.Player.Profile.Properties[1].Value = `{"endpoint_id":"endpoint-id","endpoint_org_id":"wrong","endpoint_org_id":"org-id"}`
		},
		"missing envelope": func(_ *identityVerifier, s *connect.Session) {
			s.Player.Profile.Properties = s.Player.Profile.Properties[1:]
		},
		"java marker": func(_ *identityVerifier, s *connect.Session) { s.ProtoReflect().SetUnknown(nil) },
		"passthrough": func(_ *identityVerifier, s *connect.Session) { s.Auth = &connect.Authentication{Passthrough: true} },
		"expired": func(v *identityVerifier, _ *connect.Session) {
			v.now = func() time.Time { return time.UnixMilli(1783260300000) }
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			v, s := fixtureIdentity(t, false)
			mutate(v, s)
			log, capture := capturingLogger()
			ctx := logr.NewContext(context.Background(), log)
			p := &fakeProposal{session: s}
			ph := proposalHandler{identity: v, connHandler: func(net.Conn) { t.Error("invalid identity reached handler") }}
			ph.handle(ctx, p)
			require.Len(t, p.rejections(), 1)
			require.Equal(t, "rpc error: code = Unauthenticated desc = invalid signed bedrock identity", p.rejections()[0].GetMessage())
			require.NotContains(t, capture(), "BedrockFox")
			require.NotContains(t, capture(), "2535460020985370")
		})
	}
}

func TestIdentityJavaNeedsNoMetadata(t *testing.T) {
	v := newIdentityVerifier(DefaultConfig)
	v.keys.client.Transport = identityTransport(func(*http.Request) (*http.Response, error) {
		t.Error("Java fetched Bedrock keys")
		return nil, errIdentityUnavailable
	})
	gp, err := v.gameProfile(context.Background(), &connect.Session{Id: "java-session"}, nil)
	require.NoError(t, err)
	require.Nil(t, gp)
}

func TestIdentityLinkedProfileAndProtocolIsolation(t *testing.T) {
	for _, badName := range []bool{false, true} {
		v, s := fixtureIdentity(t, false)
		data, err := os.ReadFile("internal/bedrockidentity/testdata/v1/v1-bedrock-linked-java-trusted-policy-valid.json")
		require.NoError(t, err)
		s.Player.Profile.Properties[0].Value = string(data)
		s.Player.Profile.Id = "c66dfcbc-4bd2-4a29-8c76-eadf80faa08a"
		s.Player.Profile.Name = "roboflax2"
		if badName {
			s.Player.Profile.Name = "OtherPlayer"
		}
		wire, err := connectutil.ExtractSessionPrincipalWire(s)
		require.NoError(t, err)
		gp, err := v.gameProfile(context.Background(), s, wire)
		if badName {
			require.ErrorIs(t, err, errIdentityInvalid)
		} else {
			require.NoError(t, err)
			require.Equal(t, "RoboFlax2", gp.Name)
			require.Equal(t, s.Player.Profile.Id, gp.ID.String())
		}
	}
	v, s := fixtureIdentity(t, false)
	// Mixed v1/v2 must reject before either verifier or tunnel is used.
	s.ProtoReflect().SetUnknown(nil)
	s = sessionWithEnvelope(t, s, []byte("v2-envelope"))
	p := &fakeProposal{session: s}
	h := proposalHandler{identity: v, connHandler: func(net.Conn) { t.Error("mixed protocols reached handler") }}
	h.handle(context.Background(), p)
	require.Len(t, p.rejections(), 1)
	require.Equal(t, "rpc error: code = Unauthenticated desc = invalid signed bedrock identity", p.rejections()[0].GetMessage())
	// A custom authority cannot opt into Minekube trust using reserved props.
	v, s = fixtureIdentity(t, false)
	wire, err := connectutil.ExtractSessionPrincipalWire(s)
	require.NoError(t, err)
	_, err = (*identityVerifier)(nil).gameProfile(context.Background(), s, wire)
	require.ErrorIs(t, err, errIdentityInvalid)
	v.keys = newIdentityKeys()
	v.keys.client.Transport = identityTransport(func(*http.Request) (*http.Response, error) { return nil, errIdentityUnavailable })
	_, err = v.gameProfile(context.Background(), s, wire)
	require.ErrorIs(t, err, errIdentityUnavailable)
}

func TestIdentityKeyRotationCacheAndOutage(t *testing.T) {
	k := newIdentityKeys()
	now := time.Now()
	k.now = func() time.Time { return now }
	var calls int
	var outage bool
	current := base64.StdEncoding.EncodeToString(make([]byte, 32))
	previous := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	k.client.Transport = identityTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, identityMetadataURL, r.URL.String())
		if outage {
			return nil, errIdentityUnavailable
		}
		body, _ := json.Marshal(map[string]any{"issuer": "minekube-connect", "algorithm": "Ed25519", "current_public_key": current, "previous_public_keys": []string{previous}, "cache_max_age_seconds": 10})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	keys, err := k.get(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	_, err = k.get(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	now = now.Add(6 * time.Second)
	current, previous = previous, current
	keys, err = k.get(context.Background(), true)
	require.NoError(t, err)
	require.Equal(t, byte(1), keys[0][0])
	require.Equal(t, 2, calls)
	outage = true
	now = now.Add(11 * time.Second)
	_, err = k.get(context.Background(), false)
	require.NoError(t, err, "bounded cached keys cover a transient outage")
	_, err = k.get(context.Background(), true)
	require.NoError(t, err)
	require.Equal(t, 3, calls, "failed refresh has backoff")
	now = now.Add(10 * time.Minute)
	_, err = k.get(context.Background(), false)
	require.ErrorIs(t, err, errIdentityUnavailable)
}

func TestIdentityMetadataRejectsWrongAuthorityAndRedirects(t *testing.T) {
	for _, body := range []string{
		`{"issuer":"other","algorithm":"Ed25519"}`, `{"issuer":"minekube-connect","algorithm":"RSA"}`,
		`{"issuer":"minekube-connect","algorithm":"Ed25519","current_public_key":"bad-key"}`,
		strings.Repeat("x", 64*1024+1),
	} {
		k := newIdentityKeys()
		k.client.Transport = identityTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		_, err := k.get(context.Background(), false)
		require.ErrorIs(t, err, errIdentityUnavailable)
	}
	k := newIdentityKeys()
	k.client.Transport = identityTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://attacker.test/keys"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	_, err := k.get(context.Background(), false)
	require.ErrorIs(t, err, errIdentityUnavailable)
}

type identityTunnel func(context.Context, connect.Tunnel) error

func (f identityTunnel) AcceptTunnel(c context.Context, t connect.Tunnel) error { return f(c, t) }

// Exercises the actual Watch handshake, producer proposal and WebSocket
// tunnel, using the verifier created by untouched production defaults.
func TestDefaultConnectBedrockWatchToTunnel(t *testing.T) {
	v, s := fixtureIdentity(t, false)
	var fetches atomic.Int32
	v.keys = newIdentityKeys()
	v.keys.client.Transport = identityTransport(func(*http.Request) (*http.Response, error) {
		fetches.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"issuer":"minekube-connect","algorithm":"Ed25519","current_public_key":"EvLQyZOnKxAUFaKqU2AhyOLXzPL6cHBNZjqwiLPxQy4=","cache_max_age_seconds":300}`))}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := make(chan *profile.GameProfile, 1)
	errs := make(chan error, 4)
	mux := http.NewServeMux()
	mux.HandleFunc("/watch", func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get(connect.MDPrefix + "capabilities")
		if header != bedrockidentity.Capability {
			errs <- errIdentityInvalid
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if fetches.Load() != 0 {
			errs <- errIdentityInvalid
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		socket, err := websocket.Accept(w, r, nil)
		if err != nil {
			errs <- err
			return
		}
		defer func() { _ = socket.CloseNow() }()
		payload, err := proto.Marshal(&connect.WatchResponse{Session: s})
		if err == nil {
			err = socket.Write(r.Context(), websocket.MessageBinary, payload)
		}
		if err != nil {
			errs <- err
			return
		}
		// Keep Watch open until the client shuts down. Encoding the public
		// wire directly also makes this independent of SDK server internals.
		_, _, _ = socket.Read(r.Context())
	})
	mux.Handle("/tunnel", ws.ServerOptions{}.TunnelHandler(identityTunnel(func(ctx context.Context, t connect.Tunnel) error {
		_, err := t.Write([]byte("login"))
		if err != nil {
			errs <- err
		}
		return err
	})))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	s.TunnelServiceAddr = "ws" + strings.TrimPrefix(srv.URL, "http") + "/tunnel"
	c := DefaultConfig
	c.Name = " Fixture-ENDPOINT "
	c.WatchServiceAddr = "ws" + strings.TrimPrefix(srv.URL, "http") + "/watch"
	c.TokenFilePath = filepath.Join(t.TempDir(), "connect.json")
	require.NoError(t, os.WriteFile(c.TokenFilePath, []byte(`{"token":"fixture-token"}`), 0600))
	run, err := connectClientWithIdentity(c, connHandlerFunc(func(conn net.Conn) {
		defer conn.Close()
		p, ok := netmc.Assert[proxy.GameProfileProvider](conn)
		if !ok {
			errs <- errIdentityInvalid
			return
		}
		b := make([]byte, 5)
		if _, err := io.ReadFull(conn, b); err != nil {
			errs <- err
			return
		}
		if string(b) != "login" {
			errs <- errIdentityInvalid
			return
		}
		got <- p.GameProfile()
	}), v)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- run.Start(ctx) }()
	select {
	case gp := <-got:
		require.Equal(t, "_BedrockFox", gp.Name)
		require.Len(t, gp.Properties, 1)
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("default Bedrock proposal did not reach tunnel")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch did not stop")
	}
	require.Equal(t, int32(1), fetches.Load())
}
