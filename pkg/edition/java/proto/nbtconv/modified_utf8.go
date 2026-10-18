package nbtconv

import (
	"fmt"

	"github.com/Tnze/go-mc/nbt"
)

// Java reads NBT strings with java.io.DataInputStream.readUTF, i.e. "modified
// UTF-8": NUL is encoded C0 80 and a character outside the BMP is encoded as a
// CESU-8 surrogate pair. readUTF has no 4-byte form, so a standard UTF-8
// astral character (e.g. an emoji in a MOTD, a chat message, a title or a
// tab-list header) makes a vanilla client throw
//
//	java.io.UTFDataFormatException: malformed input around byte N
//
// while decoding the packet that carries the component ("Failed to decode
// packet 'clientbound/minecraft:server_data'", and likewise every other
// component-bearing packet). Go strings are standard UTF-8, so anything Gate
// marshals from a component has to be re-encoded at this boundary.
//
// See https://docs.oracle.com/en/java/javase/21/docs/api/java.base/java/io/DataInput.html#modified-utf-8

// maxNBTDepth bounds recursion so a malformed tag cannot blow the stack.
const maxNBTDepth = 512

// EncodeModifiedUTF8 returns s encoded as Java's modified UTF-8, i.e. the bytes
// java.io.DataOutputStream.writeUTF would write (without the length prefix).
func EncodeModifiedUTF8(s string) []byte {
	// Fast path: pure ASCII (and no NUL) is byte-identical in both encodings.
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] == 0 || s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return []byte(s)
	}

	out := make([]byte, 0, len(s)+8)
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
			out = append(out,
				byte(0xe0|hi>>12), byte(0x80|hi>>6&0x3f), byte(0x80|hi&0x3f),
				byte(0xe0|lo>>12), byte(0x80|lo>>6&0x3f), byte(0x80|lo&0x3f))
		default:
			out = append(out, byte(0xe0|r>>12), byte(0x80|r>>6&0x3f), byte(0x80|r&0x3f))
		}
	}
	return out
}

// WithModifiedUTF8Strings returns a copy of bt whose string payloads and keys are
// encoded as Java's modified UTF-8, the only encoding a vanilla client can read.
// bt must hold network-format NBT (no root tag name), which is what
// SnbtToBinaryTag produces. A tag that is already pure ASCII is returned as-is.
func WithModifiedUTF8Strings(bt nbt.RawMessage) (nbt.RawMessage, error) {
	if !hasNonASCII(bt.Data) {
		return bt, nil
	}
	r := &modifiedUTF8Rewriter{in: bt.Data, out: make([]byte, 0, len(bt.Data)+8)}
	if err := r.payload(bt.Type, 0); err != nil {
		return nbt.RawMessage{}, err
	}
	if r.off != len(r.in) {
		return nbt.RawMessage{}, fmt.Errorf("nbt: %d trailing bytes after payload of tag %d", len(r.in)-r.off, bt.Type)
	}
	return nbt.RawMessage{Type: bt.Type, Data: r.out}, nil
}

func hasNonASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return true
		}
	}
	return false
}

type modifiedUTF8Rewriter struct {
	in  []byte
	off int
	out []byte
}

func (r *modifiedUTF8Rewriter) take(n int) ([]byte, error) {
	if n < 0 || r.off+n > len(r.in) {
		return nil, fmt.Errorf("nbt: truncated tag at offset %d (need %d, have %d)", r.off, n, len(r.in)-r.off)
	}
	b := r.in[r.off : r.off+n]
	r.off += n
	r.out = append(r.out, b...)
	return b, nil
}

func (r *modifiedUTF8Rewriter) skip(n int) error {
	_, err := r.take(n)
	return err
}

// count reads

func (r *modifiedUTF8Rewriter) count() (int, error) {
	if r.off+4 > len(r.in) {
		return 0, fmt.Errorf("nbt: truncated array/list length at offset %d", r.off)
	}
	n := int(int32(uint32(r.in[r.off])<<24 | uint32(r.in[r.off+1])<<16 |
		uint32(r.in[r.off+2])<<8 | uint32(r.in[r.off+3])))
	r.off += 4
	r.out = append(r.out, r.in[r.off-4:r.off]...)
	if n < 0 {
		return 0, fmt.Errorf("nbt: negative length %d", n)
	}
	return n, nil
}

// str rewrites one string (payload or compound key) to modified UTF-8.
func (r *modifiedUTF8Rewriter) str() error {
	if r.off+2 > len(r.in) {
		return fmt.Errorf("nbt: truncated string length at offset %d", r.off)
	}
	n := int(r.in[r.off])<<8 | int(r.in[r.off+1])
	r.off += 2
	if r.off+n > len(r.in) {
		return fmt.Errorf("nbt: truncated string at offset %d (len %d, have %d)", r.off, n, len(r.in)-r.off)
	}
	raw := r.in[r.off : r.off+n]
	r.off += n
	enc := EncodeModifiedUTF8(string(raw))
	if len(enc) > 0xffff {
		return fmt.Errorf("nbt: string of %d encoded bytes exceeds the NBT uint16 length", len(enc))
	}
	r.out = append(r.out, byte(len(enc)>>8), byte(len(enc)))
	r.out = append(r.out, enc...)
	return nil
}

func (r *modifiedUTF8Rewriter) payload(tag byte, depth int) error {
	if depth > maxNBTDepth {
		return fmt.Errorf("nbt: tag nesting deeper than %d", maxNBTDepth)
	}
	switch tag {
	case nbt.TagByte:
		return r.skip(1)
	case nbt.TagShort:
		return r.skip(2)
	case nbt.TagInt, nbt.TagFloat:
		return r.skip(4)
	case nbt.TagLong, nbt.TagDouble:
		return r.skip(8)
	case nbt.TagByteArray:
		n, err := r.count()
		if err != nil {
			return err
		}
		return r.skip(n)
	case nbt.TagString:
		return r.str()
	case nbt.TagList:
		if r.off >= len(r.in) {
			return fmt.Errorf("nbt: truncated list element type")
		}
		elem := r.in[r.off]
		r.off++
		r.out = append(r.out, elem)
		n, err := r.count()
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			if err := r.payload(elem, depth+1); err != nil {
				return err
			}
		}
		return nil
	case nbt.TagCompound:
		return r.compound(depth)
	case nbt.TagIntArray:
		n, err := r.count()
		if err != nil {
			return err
		}
		return r.skip(4 * n)
	case nbt.TagLongArray:
		n, err := r.count()
		if err != nil {
			return err
		}
		return r.skip(8 * n)
	default:
		return fmt.Errorf("nbt: unknown tag type %d", tag)
	}
}

// compound rewrites a compound body: repeated (type, name, payload) until TAG_End.
func (r *modifiedUTF8Rewriter) compound(depth int) error {
	for {
		if r.off >= len(r.in) {
			return fmt.Errorf("nbt: unterminated compound")
		}
		tag := r.in[r.off]
		r.off++
		r.out = append(r.out, tag)
		if tag == nbt.TagEnd {
			return nil
		}
		if err := r.str(); err != nil { // key
			return err
		}
		if err := r.payload(tag, depth+1); err != nil {
			return err
		}
	}
}
