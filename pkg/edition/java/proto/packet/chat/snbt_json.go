package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Tnze/go-mc/nbt"
	"gopkg.in/yaml.v3"
	"strings"
)

// formatSNBT adds spaces after colons that are not within quotes.
// Example: {a:1,b:hello,c:"world",d:true} -> {a: 1, b: hello, c: "world", d: true}
// This is needed because the yaml parser requires spaces after colons
func formatSNBT(snbt string) string { // TODO test / rewrite properly
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

var errSNBTInvalid = errors.New("invalid input for SNBT, must be a non-empty string starting with '{' and ending with '}'")

// SnbtToJSON converts a stringified NBT to JSON.
// Example: {a:1,b:hello,c:"world",d:true} -> {"a":1,"b":"hello","c":"world","d":true}
func SnbtToJSON(snbt string) (json.RawMessage, error) {
	// Trim whitespace, newlines, return characters, and tabs
	snbt = strings.Trim(snbt, " \n\r\t")

	// Ensure that input is not empty or trivially malformed
	if len(snbt) < 2 || !strings.HasPrefix(snbt, "{") || !strings.HasSuffix(snbt, "}") {
		// get first and last few characters of input and put ... in between
		var truncated string
		if len(snbt) > 10 {
			truncated = snbt[:5] + "..." + snbt[len(snbt)-5:]
		} else {
			truncated = snbt
		}
		return nil, fmt.Errorf("%w: but got %q", errSNBTInvalid, truncated)
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
// Example: {"a":1,"b":"hello","c":"world","d":true} -> {a:1,b:hello,c:"world",d:true}
func JsonToSNBT(j json.RawMessage) (string, error) {
	var m map[string]any
	if err := json.Unmarshal(j, &m); err != nil {
		return "", fmt.Errorf("error unmarshalling json to map: %w", err)
	}
	var b strings.Builder
	err := ConvertToSNBT(m, &b)
	return b.String(), err
}

func ConvertToSNBT(v any, b *strings.Builder) error {
	switch v := v.(type) {
	case map[string]any:
		return mapToSNBT(v, b)
	case []any:
		return sliceToSNBT(v, b)
	case string:
		if len(v) == 0 {
			// Empty strings are represented as two double quotes
			b.WriteString(`""`)
		} else {
			// Quote strings that contain spaces or special characters
			if strings.ContainsAny(v, " {}:[]/") {
				b.WriteString(fmt.Sprintf(`"%s"`, v))
			} else {
				b.WriteString(v)
			}
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
		b.WriteString(k)
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

func BinaryTagToJSON(tag *nbt.RawMessage) (json.RawMessage, error) {
	return SnbtToJSON(tag.String())
}

func SnbtToBinaryTag(snbt string) (nbt.RawMessage, error) {
	// Convert SNBT to JSON
	j, err := SnbtToJSON(snbt)
	if err != nil {
		return nbt.RawMessage{}, err
	}
	// Then convert JSON to binary tag
	return JsonToBinaryTag(j)
}

func JsonToBinaryTag(tag json.RawMessage) (nbt.RawMessage, error) {
	// Convert JSON to snbt
	snbt, err := JsonToSNBT(tag)
	if err != nil {
		return nbt.RawMessage{}, err
	}
	// Then convert snbt to bytes
	buf := new(bytes.Buffer)
	err = nbt.StringifiedMessage(snbt).MarshalNBT(buf)
	if err != nil {
		return nbt.RawMessage{}, fmt.Errorf("error marshalling snbt to binary: %w", err)
	}
	// Then convert bytes to binary tag
	var m nbt.RawMessage
	err = nbt.Unmarshal(buf.Bytes(), &m)
	if err != nil {
		return nbt.RawMessage{}, fmt.Errorf("error unmarshalling binary to binary tag: %w", err)
	}
	return m, nil
}
