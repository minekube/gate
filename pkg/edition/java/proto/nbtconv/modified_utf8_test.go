package nbtconv

// Regression coverage for NBT string encoding (task t_89b7c6c1):
// Java reads NBT strings with DataInputStream.readUTF, i.e. Modified UTF-8,
// which only accepts the 1-, 2- and 3-byte forms. A character outside the BMP
// must go out as its CESU-8 surrogate pair (and NUL as C0 80); writing it as the
// 4-byte standard UTF-8 sequence makes the vanilla client throw
//
//	java.io.UTFDataFormatException: malformed input around byte N
//
// while decoding the packet, which surfaces as
//
//	Internal Exception: io.netty.handler.codec.DecoderException:
//	Failed to decode packet 'clientbound/minecraft:server_data'
//	(likewise any other component-carrying packet: chat, title, tab list, ...)

import (
	"encoding/json"
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/Tnze/go-mc/nbt"
)

// javaReadUTF decodes a string the way java.io.DataInputStream.readUTF does.
// It is deliberately strict: 4-byte UTF-8 lead bytes are rejected, exactly as
// the JDK's implementation does.
func javaReadUTF(b []byte) (string, error) {
	var out []rune
	// collect UTF-16 code units first, like readUTF
	var units []uint16
	for i := 0; i < len(b); {
		c := b[i]
		switch c >> 4 {
		case 0, 1, 2, 3, 4, 5, 6, 7:
			units = append(units, uint16(c))
			i++
		case 12, 13:
			if i+1 >= len(b) {
				return "", fmt.Errorf("malformed input around byte %d", i)
			}
			units = append(units, uint16(c&0x1f)<<6|uint16(b[i+1]&0x3f))
			i += 2
		case 14:
			if i+2 >= len(b) {
				return "", fmt.Errorf("malformed input around byte %d", i)
			}
			units = append(units, uint16(c&0x0f)<<12|uint16(b[i+1]&0x3f)<<6|uint16(b[i+2]&0x3f))
			i += 3
		default:
			return "", fmt.Errorf("malformed input around byte %d", i)
		}
	}
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u >= 0xD800 && u <= 0xDBFF:
			if i+1 >= len(units) || units[i+1] < 0xDC00 || units[i+1] > 0xDFFF {
				out = append(out, utf8.RuneError)
				continue
			}
			out = append(out, 0x10000+(rune(u-0xD800)<<10)+rune(units[i+1]-0xDC00))
			i++
		case u >= 0xDC00 && u <= 0xDFFF:
			out = append(out, utf8.RuneError)
		default:
			out = append(out, rune(u))
		}
	}
	return string(out), nil
}

// walkNBTStrings visits every string (keys included) in a network-format NBT tag,
// asserting each one is readable by java.io.DataInputStream.readUTF.
func walkNBTStrings(t *testing.T, b []byte) []string {
	t.Helper()
	var got []string
	off := 0
	var readString func() string
	readString = func() string {
		if off+2 > len(b) {
			t.Fatalf("truncated string length at %d", off)
		}
		n := int(b[off])<<8 | int(b[off+1])
		off += 2
		if off+n > len(b) {
			t.Fatalf("truncated string at %d (len %d, have %d)", off, n, len(b)-off)
		}
		raw := b[off : off+n]
		off += n
		s, err := javaReadUTF(raw)
		if err != nil {
			t.Fatalf("java UTFDataFormatException: %v (bytes % x)", err, raw)
		}
		got = append(got, s)
		return s
	}
	var payload func(tag byte)
	var compoundBody func()
	payload = func(tag byte) {
		switch tag {
		case 1:
			off++
		case 2:
			off += 2
		case 3:
			off += 4
		case 4, 6:
			off += 8
		case 5:
			off += 4
		case 7:
			n := int(int32(uint32(b[off])<<24 | uint32(b[off+1])<<16 | uint32(b[off+2])<<8 | uint32(b[off+3])))
			off += 4 + n
		case 8:
			readString()
		case 9:
			et := b[off]
			off++
			n := int(int32(uint32(b[off])<<24 | uint32(b[off+1])<<16 | uint32(b[off+2])<<8 | uint32(b[off+3])))
			off += 4
			for i := 0; i < n; i++ {
				if et == 10 {
					compoundBody()
				} else {
					payload(et)
				}
			}
		case 10:
			compoundBody()
		case 11:
			n := int(int32(uint32(b[off])<<24 | uint32(b[off+1])<<16 | uint32(b[off+2])<<8 | uint32(b[off+3])))
			off += 4 + 4*n
		case 12:
			n := int(int32(uint32(b[off])<<24 | uint32(b[off+1])<<16 | uint32(b[off+2])<<8 | uint32(b[off+3])))
			off += 4 + 8*n
		default:
			t.Fatalf("unknown tag %d at %d", tag, off)
		}
	}
	compoundBody = func() {
		for {
			tag := b[off]
			off++
			if tag == 0 {
				return
			}
			readString() // key
			payload(tag)
		}
	}
	if b[0] != 10 {
		t.Fatalf("expected compound root, got tag %d", b[0])
	}
	off = 1
	compoundBody()
	if off != len(b) {
		t.Fatalf("%d trailing bytes after NBT root", len(b)-off)
	}
	return got
}

func TestJsonToBinaryTagWritesJavaReadableStrings(t *testing.T) {
	cases := map[string]struct {
		json string
		want string
	}{
		"plain":            {`{"text":"hello"}`, "hello"},
		"bmp accents":      {`{"text":"Ünïcödé ⚔"}`, "Ünïcödé ⚔"},
		"emoji":            {`{"text":"§cRed 😀"}`, "§cRed 😀"},
		"astral in child":  {`{"text":"","extra":[{"text":"🎮 play"}]}`, "🎮 play"},
		"astral in hover":  {`{"text":"q","hover_event":{"action":"show_text","contents":{"text":"✨"}}}`, "✨"},
		"astral in grapheme": {`{"text":"family 👨‍👩‍👧‍👦"}`, "family 👨‍👩‍👧‍👦"},
		"astral U+20000":   {`{"text":"\ud840\udc00"}`, "\U00020000"},
		"astral U+1D11E":   {`{"text":"\ud834\udd1e"}`, "\U0001D11E"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var raw json.RawMessage = []byte(tc.json)
			bt, err := JsonToBinaryTag(raw)
			if err != nil {
				t.Fatalf("JsonToBinaryTag: %v", err)
			}
			// WriteBinaryTag writes the root type byte followed by bt.Data.
			full := append([]byte{bt.Type}, bt.Data...)
			// walkNBTStrings fails the test on a non-Java-readable string.
			strings := walkNBTStrings(t, full)
			found := false
			for _, s := range strings {
				if s == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("did not find %q among decoded strings %q", tc.want, strings)
			}
			// The wire bytes must be exactly java.io.DataOutputStream.writeUTF's output.
			want := javaWriteUTF(tc.want)
			if !containsBytes(bt.Data, want) {
				t.Fatalf("NBT does not contain writeUTF(%q) = % x", tc.want, want)
			}
		})
	}
}

// javaWriteUTF mirrors java.io.DataOutputStream.writeUTF (without the length prefix).
func javaWriteUTF(s string) []byte {
	var out []byte
	for _, r := range s {
		switch {
		case r != 0 && r < 0x80:
			out = append(out, byte(r))
		case r < 0x800:
			out = append(out, byte(0xc0|r>>6), byte(0x80|r&0x3f))
		case r >= 0x10000:
			r -= 0x10000
			hi := rune(0xd800) + (r >> 10)
			lo := rune(0xdc00) + (r & 0x3ff)
			out = append(out, byte(0xe0|hi>>12), byte(0x80|hi>>6&0x3f), byte(0x80|hi&0x3f))
			out = append(out, byte(0xe0|lo>>12), byte(0x80|lo>>6&0x3f), byte(0x80|lo&0x3f))
		default:
			out = append(out, byte(0xe0|r>>12), byte(0x80|r>>6&0x3f), byte(0x80|r&0x3f))
		}
	}
	return out
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

var _ = nbt.RawMessage{}
