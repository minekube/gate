package proxy

import (
	"bytes"
	"net"
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/bungeecord"
	"go.minekube.com/gate/pkg/gate/proto"
)

// brandPayload frames a brand the way the protocol frames a brand message
// payload: a raw string before 1.8, a length-prefixed string from 1.8 on.
func brandPayload(t *testing.T, p proto.Protocol, brand string) []byte {
	t.Helper()
	if p.GreaterEqual(version.Minecraft_1_8) {
		buf := new(bytes.Buffer)
		if err := util.WriteString(buf, brand); err != nil {
			t.Fatalf("encode %q: %v", brand, err)
		}
		return buf.Bytes()
	}
	return []byte(brand)
}

// brandHarness wires a real backend session handler to fake client and backend
// connections, so a test reads the exact payload bytes Gate writes to the player.
type brandHarness struct {
	proxy      *Proxy
	player     *connectedPlayer
	serverConn *serverConnection
	playerConn *testMinecraftConn
	events     event.Manager
}

func newBrandHarness(t *testing.T, cfg *config.Config, protocol proto.Protocol) *brandHarness {
	t.Helper()

	events := event.New()
	playerConn := &testMinecraftConn{protocol: protocol}
	backendConn := &testMinecraftConn{protocol: protocol}
	player := &connectedPlayer{
		MinecraftConn: playerConn,
		log:           logr.Discard(),
	}
	px := &Proxy{event: events, cfg: cfg}
	player.sessionHandlerDeps = &sessionHandlerDeps{proxy: px}

	server := newRegisteredServer(NewServerInfo("purpur", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 25565}))
	serverConn := newServerConnection(server, nil, player)
	serverConn.connection = backendConn

	return &brandHarness{
		proxy:      px,
		player:     player,
		serverConn: serverConn,
		playerConn: playerConn,
		events:     events,
	}
}

// playBrand delivers a backend brand message through the play-phase handler.
func (h *brandHarness) playBrand(data []byte) {
	handler := &backendPlaySessionHandler{
		serverConn:                 h.serverConn,
		bungeeCordMessageResponder: bungeecord.NopMessageResponder,
		log:                        logr.Discard(),
	}
	handler.handlePluginMessage(&plugin.Message{Channel: plugin.BrandChannel, Data: data}, nil)
}

// configBrand delivers a backend brand message through the config-phase handler.
func (h *brandHarness) configBrand(data []byte) {
	handler := &backendConfigSessionHandler{serverConn: h.serverConn, log: logr.Discard()}
	handler.handlePluginMessage(nil, &plugin.Message{Channel: plugin.BrandChannel, Data: data})
}

// emitted returns the payload of the single plugin message written to the player.
func (h *brandHarness) emitted(t *testing.T) []byte {
	t.Helper()
	if len(h.playerConn.writtenPackets) != 1 {
		t.Fatalf("expected exactly one packet written to the player, got %d (%#v)",
			len(h.playerConn.writtenPackets), h.playerConn.writtenPackets)
	}
	msg, ok := h.playerConn.writtenPackets[0].(*plugin.Message)
	if !ok {
		t.Fatalf("expected a plugin message, got %T", h.playerConn.writtenPackets[0])
	}
	return msg.Data
}

func configuredSuffix(s string) *config.Config {
	cfg := config.DefaultConfig
	cfg.BrandSuffix = &s
	return &cfg
}

func defaultConfig() *config.Config {
	cfg := config.DefaultConfig
	cfg.BrandSuffix = nil
	return &cfg
}

// TestBackendBrandRewriteMatrix drives the play-phase brand path with the real
// session handler and asserts the emitted payload bytes for every combination
// the card requires: both protocol framings times the default suffix, an empty
// suffix, a per-session event override and no event handler at all.
func TestBackendBrandRewriteMatrix(t *testing.T) {
	const backendBrand = "vanilla"

	protocols := []struct {
		name string
		p    proto.Protocol
	}{
		{"pre-1.8", version.Minecraft_1_7_6.Protocol},
		{"1.8+", version.Minecraft_1_21.Protocol},
	}

	type wantKind int
	const (
		wantBrand wantKind = iota // a rewritten brand string
		wantAsIs                  // the backend's own bytes, untouched
	)

	cases := []struct {
		name      string
		cfg       func() *config.Config
		subscribe func(*brandHarness)
		want      string
		kind      wantKind
	}{
		{
			name: "default config appends the shipped suffix",
			cfg:  defaultConfig,
			want: backendBrand + " (Gate by Minekube)",
		},
		{
			name: "explicit shipped suffix is the same bytes",
			cfg:  func() *config.Config { return configuredSuffix(config.DefaultBrandSuffix) },
			want: backendBrand + " (Gate by Minekube)",
		},
		{
			name: "empty suffix sends the backend brand as-is",
			cfg:  func() *config.Config { return configuredSuffix("") },
			kind: wantAsIs,
		},
		{
			name: "custom suffix",
			cfg:  func() *config.Config { return configuredSuffix(" (My Network)") },
			want: backendBrand + " (My Network)",
		},
		{
			name: "event override suppresses the suffix",
			cfg:  defaultConfig,
			subscribe: func(h *brandHarness) {
				event.Subscribe(h.events, 0, func(e *ServerBrandRewriteEvent) {
					e.SetBrand(e.OriginalBrand())
				})
			},
			kind: wantAsIs,
		},
		{
			name: "event override replaces the brand",
			cfg:  defaultConfig,
			subscribe: func(h *brandHarness) {
				event.Subscribe(h.events, 0, func(e *ServerBrandRewriteEvent) {
					e.SetBrand("MyServer")
				})
			},
			want: "MyServer",
		},
		{
			name:      "no event handler keeps the configured suffix",
			cfg:       func() *config.Config { return configuredSuffix(" (My Network)") },
			subscribe: func(*brandHarness) {},
			want:      backendBrand + " (My Network)",
		},
	}

	for _, protoCase := range protocols {
		t.Run(protoCase.name, func(t *testing.T) {
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					h := newBrandHarness(t, c.cfg(), protoCase.p)
					if c.subscribe != nil {
						c.subscribe(h)
					}

					backendData := brandPayload(t, protoCase.p, backendBrand)
					h.playBrand(backendData)
					got := h.emitted(t)

					want := backendData
					if c.kind == wantBrand {
						want = brandPayload(t, protoCase.p, c.want)
					}
					if !bytes.Equal(got, want) {
						t.Fatalf("emitted payload = % x, want % x", got, want)
					}
				})
			}
		})
	}
}

// TestBackendConfigBrandRewrite proves the config-phase brand path (1.20.2+)
// honours the same config and the same per-session override, so a client that
// negotiates the brand in the config phase cannot keep the old branding.
func TestBackendConfigBrandRewrite(t *testing.T) {
	p := version.Minecraft_1_21.Protocol
	const backendBrand = "vanilla"

	t.Run("default config appends the shipped suffix", func(t *testing.T) {
		h := newBrandHarness(t, defaultConfig(), p)
		h.configBrand(brandPayload(t, p, backendBrand))
		if got, want := h.emitted(t), brandPayload(t, p, backendBrand+" (Gate by Minekube)"); !bytes.Equal(got, want) {
			t.Fatalf("emitted payload = % x, want % x", got, want)
		}
	})

	t.Run("empty suffix sends the backend brand as-is", func(t *testing.T) {
		h := newBrandHarness(t, configuredSuffix(""), p)
		data := brandPayload(t, p, backendBrand)
		h.configBrand(data)
		if got := h.emitted(t); !bytes.Equal(got, data) {
			t.Fatalf("emitted payload = % x, want the backend's % x", got, data)
		}
	})

	t.Run("event override suppresses the suffix", func(t *testing.T) {
		h := newBrandHarness(t, defaultConfig(), p)
		event.Subscribe(h.events, 0, func(e *ServerBrandRewriteEvent) {
			e.SetBrand(e.OriginalBrand())
		})
		data := brandPayload(t, p, backendBrand)
		h.configBrand(data)
		if got := h.emitted(t); !bytes.Equal(got, data) {
			t.Fatalf("emitted payload = % x, want the backend's % x", got, data)
		}
	})
}

// TestBackendBrandRewriteEventObservability proves a handler always sees what the
// backend actually reported (even when it replaces the brand) and the player the
// session belongs to, which is what an embedding proxy keys its per-endpoint
// policy on.
func TestBackendBrandRewriteEventObservability(t *testing.T) {
	p := version.Minecraft_1_21.Protocol
	h := newBrandHarness(t, defaultConfig(), p)

	var (
		fired    int
		original string
		planned  string
		eventFor Player
	)
	event.Subscribe(h.events, 0, func(e *ServerBrandRewriteEvent) {
		fired++
		original = e.OriginalBrand()
		planned = e.Brand()
		eventFor = e.Player()
		e.SetBrand("replaced")
	})

	h.playBrand(brandPayload(t, p, "paper"))

	if fired != 1 {
		t.Fatalf("event fired %d times, want 1", fired)
	}
	if original != "paper" {
		t.Fatalf("OriginalBrand() = %q, want %q", original, "paper")
	}
	if planned != "paper (Gate by Minekube)" {
		t.Fatalf("Brand() before override = %q, want %q", planned, "paper (Gate by Minekube)")
	}
	if eventFor != h.player {
		t.Fatalf("Player() = %#v, want the session's player", eventFor)
	}
	if got, want := h.emitted(t), brandPayload(t, p, "replaced"); !bytes.Equal(got, want) {
		t.Fatalf("emitted payload = % x, want % x", got, want)
	}
}

// TestBackendEmptyBrandAddsNothing proves the empty-brand rule: no rewrite
// configured for the session means the backend's (empty) brand is forwarded
// untouched, so a suppressed rewrite never invents a brand packet payload.
func TestBackendEmptyBrandAddsNothing(t *testing.T) {
	p := version.Minecraft_1_21.Protocol

	t.Run("empty backend brand with default config keeps today's suffix", func(t *testing.T) {
		h := newBrandHarness(t, defaultConfig(), p)
		h.playBrand(nil)
		if got, want := h.emitted(t), brandPayload(t, p, " (Gate by Minekube)"); !bytes.Equal(got, want) {
			t.Fatalf("emitted payload = % x, want % x", got, want)
		}
	})

	t.Run("empty backend brand with an empty suffix adds nothing", func(t *testing.T) {
		h := newBrandHarness(t, configuredSuffix(""), p)
		h.playBrand([]byte{})
		if got := h.emitted(t); len(got) != 0 {
			t.Fatalf("emitted payload = % x, want none", got)
		}
	})

	t.Run("empty backend brand with a suppressed event adds nothing", func(t *testing.T) {
		h := newBrandHarness(t, defaultConfig(), p)
		event.Subscribe(h.events, 0, func(e *ServerBrandRewriteEvent) {
			e.SetBrand(e.OriginalBrand())
		})
		h.playBrand([]byte{})
		if got := h.emitted(t); len(got) != 0 {
			t.Fatalf("emitted payload = % x, want none", got)
		}
	})
}
