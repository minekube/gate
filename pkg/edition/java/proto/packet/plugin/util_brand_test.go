package plugin

import (
	"bytes"
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// framed encodes want the way the protocol frames a brand message payload: a
// raw string before 1.8 and a VarInt-length-prefixed string from 1.8 on. The
// test states the framing itself instead of deriving it from the code under
// test, so a regression in either branch fails on the bytes.
func framed(t *testing.T, p proto.Protocol, want string) []byte {
	t.Helper()
	if p.GreaterEqual(version.Minecraft_1_8) {
		buf := new(bytes.Buffer)
		if err := util.WriteString(buf, want); err != nil {
			t.Fatalf("encode %q: %v", want, err)
		}
		return buf.Bytes()
	}
	return []byte(want)
}

// TestRewriteMinecraftBrandEmitsExpectedPayloadBytes pins the exact payload
// bytes Gate sends for a backend brand, across the two protocol framings and
// the configurable suffix:
//
//   - the default suffix keeps the bytes Gate has always emitted,
//   - an explicit brand (what config.brandSuffix and ServerBrandRewriteEvent
//     resolve to) is written verbatim,
//   - an empty brand still produces a well-formed payload per framing (the
//     length prefix before 1.8 vs. nothing after it).
func TestRewriteMinecraftBrandEmitsExpectedPayloadBytes(t *testing.T) {
	const backendBrand = "vanilla"
	const defaultBrand = backendBrand + " (Gate by Minekube)"

	protocols := []struct {
		name string
		p    proto.Protocol
	}{
		{"1.7.10", version.Minecraft_1_7_6.Protocol},
		{"1.8", version.Minecraft_1_8.Protocol},
		{"1.21", version.Minecraft_1_21.Protocol},
	}

	for _, protoCase := range protocols {
		t.Run(protoCase.name, func(t *testing.T) {
			input := func() *Message {
				return &Message{Channel: BrandChannel, Data: framed(t, protoCase.p, backendBrand)}
			}

			cases := []struct {
				name string
				got  *Message
				want string
			}{
				{
					name: "default suffix",
					got:  RewriteMinecraftBrand(input(), protoCase.p),
					want: defaultBrand,
				},
				{
					name: "explicit empty brand disables the rewrite",
					got:  RewriteMinecraftBrandTo(input(), protoCase.p, ""),
					want: "",
				},
				{
					name: "backend brand passed through unchanged",
					got:  RewriteMinecraftBrandTo(input(), protoCase.p, backendBrand),
					want: backendBrand,
				},
				{
					name: "custom suffix",
					got:  RewriteMinecraftBrandTo(input(), protoCase.p, backendBrand+" (My Network)"),
					want: backendBrand + " (My Network)",
				},
				{
					name: "replaced brand",
					got:  RewriteMinecraftBrandTo(input(), protoCase.p, "MyServer"),
					want: "MyServer",
				},
			}

			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					if c.got == nil {
						t.Fatal("expected a rewritten message, got nil")
					}
					if c.got.Channel != BrandChannel {
						t.Fatalf("channel = %q, want %q", c.got.Channel, BrandChannel)
					}
					wantBytes := framed(t, protoCase.p, c.want)
					if !bytes.Equal(c.got.Data, wantBytes) {
						t.Fatalf("payload bytes = % x, want % x", c.got.Data, wantBytes)
					}
				})
			}
		})
	}
}

// TestRewriteMinecraftBrandEmptyBackendBrand covers the backend that sends no
// usable brand: today's default appends the suffix to nothing (" (Gate by
// Minekube)"), while a suppressed rewrite adds nothing at all.
func TestRewriteMinecraftBrandEmptyBackendBrand(t *testing.T) {
	protocols := []struct {
		name string
		p    proto.Protocol
	}{
		{"1.7.10", version.Minecraft_1_7_6.Protocol},
		{"1.21", version.Minecraft_1_21.Protocol},
	}

	for _, protoCase := range protocols {
		t.Run(protoCase.name, func(t *testing.T) {
			empty := func() *Message {
				return &Message{Channel: BrandChannel, Data: framed(t, protoCase.p, "")}
			}

			// Today's behaviour, kept byte-identical: the suffix is appended
			// to an empty brand.
			got := RewriteMinecraftBrand(empty(), protoCase.p)
			want := framed(t, protoCase.p, " (Gate by Minekube)")
			if !bytes.Equal(got.Data, want) {
				t.Fatalf("default payload bytes = % x, want % x", got.Data, want)
			}

			// A suppressed rewrite of an empty brand adds nothing: the empty
			// payload is framed exactly like the backend framed it.
			got = RewriteMinecraftBrandTo(empty(), protoCase.p, "")
			if !bytes.Equal(got.Data, empty().Data) {
				t.Fatalf("suppressed payload bytes = % x, want the backend's % x", got.Data, empty().Data)
			}
		})
	}
}

// TestRewriteMinecraftBrandLeavesOtherMessagesAlone proves the helper is a
// no-op for nil and non-brand plugin messages, and never mutates its input.
func TestRewriteMinecraftBrandLeavesOtherMessagesAlone(t *testing.T) {
	if got := RewriteMinecraftBrand(nil, version.Minecraft_1_21.Protocol); got != nil {
		t.Fatalf("nil message = %#v, want nil", got)
	}
	if got := RewriteMinecraftBrandTo(nil, version.Minecraft_1_21.Protocol, "x"); got != nil {
		t.Fatalf("nil message = %#v, want nil", got)
	}

	other := &Message{Channel: "minecraft:register", Data: []byte("axiom:hello")}
	if got := RewriteMinecraftBrand(other, version.Minecraft_1_21.Protocol); got != other {
		t.Fatal("non-brand message must be returned as-is")
	}
	if got := RewriteMinecraftBrandTo(other, version.Minecraft_1_21.Protocol, "x"); got != other {
		t.Fatal("non-brand message must be returned as-is")
	}

	in := &Message{Channel: BrandChannel, Data: framed(t, version.Minecraft_1_21.Protocol, "vanilla")}
	before := append([]byte(nil), in.Data...)
	_ = RewriteMinecraftBrand(in, version.Minecraft_1_21.Protocol)
	_ = RewriteMinecraftBrandTo(in, version.Minecraft_1_21.Protocol, "other")
	if !bytes.Equal(in.Data, before) {
		t.Fatalf("input payload mutated: % x, want % x", in.Data, before)
	}
}
