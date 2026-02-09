package nbtconv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Tnze/go-mc/nbt"
	"gopkg.in/yaml.v3"
)

// decodeCESU8 converts a string containing CESU-8 encoded surrogate pairs
// into valid UTF-8. Java's Modified UTF-8 (used in NBT) encodes supplementary
// Unicode characters (U+10000 and above, e.g. emoji) as a pair of 3-byte
// CESU-8 sequences representing a UTF-16 surrogate pair, which is invalid UTF-8.
// This function finds those paired surrogates and replaces them with the correct
// 4-byte UTF-8 encoding. Unpaired surrogates and any other invalid UTF-8
// sequences are replaced with U+FFFD.
func decodeCESU8(s string) string {
	b := []byte(s)
	n := len(b)
	if n == 0 {
		return s
	}

	// Quick check: if already valid UTF-8, no CESU-8 surrogates are present.
	if utf8.Valid(b) {
		return s
	}

	var out []byte
	for i := 0; i < n; {
		// Check for a CESU-8 surrogate pair: two 3-byte sequences
		// High surrogate: ED [A0-AF] [80-BF]  (U+D800..U+DBFF)
		// Low surrogate:  ED [B0-BF] [80-BF]  (U+DC00..U+DFFF)
		if i+5 < n &&
			b[i] == 0xED && b[i+1] >= 0xA0 && b[i+1] <= 0xAF && b[i+2] >= 0x80 && b[i+2] <= 0xBF &&
			b[i+3] == 0xED && b[i+4] >= 0xB0 && b[i+4] <= 0xBF && b[i+5] >= 0x80 && b[i+5] <= 0xBF {
			// Decode high surrogate from 3-byte CESU-8: (b0 & 0x0F)<<12 | (b1 & 0x3F)<<6 | (b2 & 0x3F)
			high := rune(b[i]&0x0F)<<12 | rune(b[i+1]&0x3F)<<6 | rune(b[i+2]&0x3F)
			// Decode low surrogate
			low := rune(b[i+3]&0x0F)<<12 | rune(b[i+4]&0x3F)<<6 | rune(b[i+5]&0x3F)
			// Combine into supplementary codepoint
			cp := 0x10000 + (high-0xD800)<<10 + (low - 0xDC00)

			if out == nil {
				out = make([]byte, 0, n)
				out = append(out, b[:i]...)
			}
			var buf [4]byte
			utf8.EncodeRune(buf[:], cp)
			out = append(out, buf[:]...)
			i += 6
			continue
		}

		// Try to decode a valid UTF-8 rune at this position
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			// Invalid byte sequence — could be an unpaired surrogate,
			// a truncated multi-byte sequence, or a stray byte.
			// Replace with U+FFFD.
			if out == nil {
				out = make([]byte, 0, n)
				out = append(out, b[:i]...)
			}
			out = append(out, []byte(string(utf8.RuneError))...)
			if size == 0 {
				i++ // avoid infinite loop on zero-width error
			} else {
				i += size
			}
			continue
		}

		// Valid rune
		if out != nil {
			out = append(out, b[i:i+size]...)
		}
		i += size
	}

	if out == nil {
		return s
	}
	return string(out)
}

// formatSNBT adds spaces after colons and commas that are not within quotes,
// and quotes empty keys so the YAML parser can handle them.
// Example: {a:1,b:hello,c:"world",d:true} -> {a: 1, b: hello, c: "world", d: true}
// Example: {:"",text:hi} -> {"": "", text: hi}
// This is needed because the yaml parser requires spaces after colons and
// cannot parse empty keys in flow mappings.
// It correctly handles escaped quotes (\") and (\\) inside quoted strings,
// as well as single-quoted strings with (\') escapes as produced by go-mc.
func formatSNBT(snbt string) string {
	var result strings.Builder
	result.Grow(len(snbt) + len(snbt)/4) // estimate extra space for added spaces
	var quoteChar byte                   // 0 = not in quotes, '"' or '\'' = in that quote type
	hasKeyContent := false               // whether we've seen key content since last '{' or ','

	for i := 0; i < len(snbt); i++ {
		c := snbt[i]
		if quoteChar != 0 {
			// Inside a quoted string
			if c == '\\' && i+1 < len(snbt) {
				// Escaped character — write both bytes and skip the next one
				result.WriteByte(c)
				i++
				result.WriteByte(snbt[i])
				continue
			}
			if c == quoteChar {
				// End of quoted string
				quoteChar = 0
			}
			result.WriteByte(c)
			continue
		}
		// Outside quotes
		switch c {
		case '"', '\'':
			quoteChar = c
			hasKeyContent = true
		case '{':
			hasKeyContent = false
		case ',':
			hasKeyContent = false
			result.WriteByte(c)
			result.WriteByte(' ')
			continue
		case ':':
			if !hasKeyContent {
				// Empty key — insert a quoted empty string so YAML can parse it
				result.WriteString(`""`)
			}
			hasKeyContent = false
			result.WriteByte(c)
			result.WriteByte(' ')
			continue
		default:
			if c != ' ' && c != '\t' {
				hasKeyContent = true
			}
		}
		result.WriteByte(c)
	}

	return result.String()
}

// SnbtToJSON converts a stringified NBT to JSON.
// Example: {a:1,b:hello,c:"world",d:true} -> {"a":1,"b":"hello","c":"world","d":true}
func SnbtToJSON(snbt string) (json.RawMessage, error) {
	// Decode CESU-8 surrogate pairs from Java's Modified UTF-8 (used in NBT)
	// into valid UTF-8 before any parsing.
	snbt = decodeCESU8(snbt)

	// Trim whitespace, newlines, return characters, and tabs
	snbt = strings.TrimSpace(snbt)

	// Ensure that input is not empty or trivially malformed
	if len(snbt) < 2 || !strings.HasPrefix(snbt, "{") || !strings.HasSuffix(snbt, "}") {
		if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
			// get first and last few characters of input and put ... in between
			var truncated string
			if len(snbt) > 10 {
				truncated = snbt[:5] + "..." + snbt[len(snbt)-5:]
			} else {
				truncated = snbt
			}
			slog.Debug("got non-object snbt", "snbt", truncated)
		}

		// just a json string
		return json.RawMessage(strconv.Quote(snbt)), nil
	}

	// Add spaces after colons that are not within quotes
	snbt = formatSNBT(snbt)

	// Parse non-standard json with yaml, which is a superset of json.
	// We use YAML parser, since it's a superset of JSON and quotes are optional.
	type M map[string]any
	var m M
	if err := yaml.Unmarshal([]byte(snbt), &m); err != nil {
		return nil, fmt.Errorf("error unmarshalling snbt to yaml: %w", err)
	}
	// Marshal back to JSON
	j, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("error marshalling yaml to json: %w", err)
	}
	return j, nil
}

// JsonToSNBT converts a JSON to stringified NBT.
// Example: {"a":1,"b":"hello","c":"world","d":true} -> {a:1,b:"hello",c:"world",d:1}
func JsonToSNBT(j json.RawMessage) (string, error) {
	var m map[string]any
	if err := json.Unmarshal(j, &m); err != nil {
		return "", fmt.Errorf("error unmarshalling json to map: %w", err)
	}
	var b strings.Builder
	err := ConvertToSNBT(m, &b)
	return b.String(), err
}

// ConvertToSNBT converts a map[string]any to stringified NBT by writing to a strings.Builder.
func ConvertToSNBT(v any, b *strings.Builder) error {
	switch v := v.(type) {
	case map[string]any:
		return mapToSNBT(v, b)
	case []any:
		return sliceToSNBT(v, b)
	case string:
		writeStr(v, b, false)
	case bool:
		if v {
			b.WriteString("1")
		} else {
			b.WriteString("0")
		}
	default:
		b.WriteString(fmt.Sprintf("%v", v))
	}
	return nil
}

func mapToSNBT(m map[string]any, b *strings.Builder) error {
	b.WriteString("{")
	sep := ""
	for k, v := range m {
		b.WriteString(sep)
		writeStr(k, b, true)
		b.WriteString(":")
		err := ConvertToSNBT(v, b)
		if err != nil {
			return err
		}
		sep = ","
	}
	b.WriteString("}")
	return nil
}

var notToEscapeStrRe = regexp.MustCompile(`^[a-zA-Z0-9_\-.+]+$`)

func writeStr(s string, b *strings.Builder, isKey bool) {
	if isKey && strings.TrimSpace(s) != "" && notToEscapeStrRe.MatchString(s) {
		b.WriteString(s)
	} else {
		// Quote empty strings or that contain special characters.
		// We cannot use strconv.Quote because we only want to escape " characters,
		// but not \n, \t, etc.
		escapedStr := strings.ReplaceAll(s, `"`, `\"`)
		b.WriteString(`"` + escapedStr + `"`)
	}
}

func sliceToSNBT(s []any, b *strings.Builder) error {
	b.WriteString("[")
	for i, item := range s {
		if i != 0 {
			b.WriteString(",")
		}
		err := ConvertToSNBT(item, b)
		if err != nil {
			return err
		}
	}
	b.WriteString("]")
	return nil
}

// BinaryTagToJSON converts a binary tag to JSON.
func BinaryTagToJSON(tag *nbt.RawMessage) (json.RawMessage, error) {
	return SnbtToJSON(tag.String())
}

// SnbtToBinaryTag converts a stringified NBT to binary tag.
func SnbtToBinaryTag(snbt string) (nbt.RawMessage, error) {
	// Then convert snbt to bytes
	buf := new(bytes.Buffer)
	err := nbt.StringifiedMessage(snbt).MarshalNBT(buf)
	if err != nil {
		return nbt.RawMessage{}, fmt.Errorf("error marshalling snbt to binary: %w", err)
	}

	rd := io.MultiReader(
		bytes.NewReader([]byte{10}), // type: TagCompound
		buf,                         // struct fields: Data
		bytes.NewReader([]byte{0}),  // end TagCompound
	)

	// This is an example the structure of a binary tag for a kick message:
	// It is a compound tag with 3 tags:
	// - color: red
	// - bold: true
	// - text: KickAll
	//
	// As stringified NBT (snbt) it looks like this:
	// {color:red,bold:1,text:KickAll}
	//
	// The first TagByte (1, 0) represents the type of the tag (TagByte) and the name of the tag (empty).

	//return nbt.RawMessage{
	//	Type: nbt.TagCompound,
	//	Data: []byte{
	//		//10, // type: TagCompound (held by Type field)
	//		//0, 0, // Named tag string length empty (disabled in network format)
	//
	//		8,                            // type: TagString
	//		0, 5, 99, 111, 108, 111, 114, // string=color length=5
	//		0, 3, 114, 101, 100, // string=red length=3
	//
	//		1,                       // type: TagByte
	//		0, 4, 98, 111, 108, 100, // string=bold length=4
	//		1, // TagByte true
	//
	//		8,                        // type: TagString
	//		0, 4, 116, 101, 120, 116, // string=text length=4
	//		0, 7, 75, 105, 99, 107, 65, 108, 108, // string=KickAll length=7
	//
	//		0, // End TagCompound
	//	},
	//}, nil

	// Then convert bytes to binary tag
	dec := nbt.NewDecoder(rd)
	// Remove index 1 and 2 from buf.Bytes() (which are the length of the tag name)
	// because we don't want them in network format
	dec.NetworkFormat(true)

	var m nbt.RawMessage
	if _, err = dec.Decode(&m); err != nil {
		return m, fmt.Errorf("error decoding snbt to binary tag: %w", err)
	}
	return m, nil
}

// JsonToBinaryTag converts a JSON to binary tag.
//
// Note that type information such as boolean is lost in the conversion, since
// SNBT uses 1 and 0 byte values for booleans which are not distinguishable from
// JSON numbers.
//
// Example: {"a":1,"b":"hello","c":"world","d":true} -> {a:1,b:hello,c:"world",d:1}
func JsonToBinaryTag(j json.RawMessage) (nbt.RawMessage, error) {
	snbt, err := JsonToSNBT(j)
	if err != nil {
		return nbt.RawMessage{}, err
	}
	return SnbtToBinaryTag(snbt)
}
