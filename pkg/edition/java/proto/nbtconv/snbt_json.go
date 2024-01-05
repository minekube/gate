package nbtconv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Tnze/go-mc/nbt"
	"gopkg.in/yaml.v3"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
)

// formatSNBT adds spaces after colons that are not within quotes.
// Example: {a:1,b:hello,c:"world",d:true} -> {a: 1, b: hello, c: "world", d: true}
// This is needed because the yaml parser requires spaces after colons
func formatSNBT(snbt string) string { // TODO test properly
	var result strings.Builder
	inQuotes := false

	for i := 0; i < len(snbt); i++ {
		switch snbt[i] {
		case '"':
			inQuotes = !inQuotes
		case ':', ',':
			if !inQuotes {
				result.WriteByte(snbt[i])
				result.WriteByte(' ')
				continue
			}
		}
		result.WriteByte(snbt[i])
	}

	return result.String()
}

// SnbtToJSON converts a stringified NBT to JSON.
// Example: {a:1,b:hello,c:"world",d:true} -> {"a":1,"b":"hello","c":"world","d":true}
func SnbtToJSON(snbt string) (json.RawMessage, error) {
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
		return m, fmt.Errorf("error decoding binary tag: %w", err)
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
