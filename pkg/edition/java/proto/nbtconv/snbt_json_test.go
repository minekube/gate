//go:build go1.18

package nbtconv

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeCESU8(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain ascii",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "valid utf-8 passthrough",
			input: "café ñ 日本語",
			want:  "café ñ 日本語",
		},
		{
			name: "U+1F600 grinning face - CESU-8 surrogate pair",
			// U+1F600 → UTF-16: D83D DE00
			// CESU-8 high surrogate D83D: ED A0 BD
			// CESU-8 low surrogate  DE00: ED B8 80
			input: "hello \xED\xA0\xBD\xED\xB8\x80 world",
			want:  "hello 😀 world",
		},
		{
			name: "U+1F4A9 pile of poo",
			// U+1F4A9 → UTF-16: D83D DCA9
			// High: ED A0 BD, Low: ED B2 A9
			input: "\xED\xA0\xBD\xED\xB2\xA9",
			want:  "💩",
		},
		{
			name: "multiple surrogate pairs",
			// Two emoji in a row: U+1F600 then U+1F4A9
			input: "\xED\xA0\xBD\xED\xB8\x80\xED\xA0\xBD\xED\xB2\xA9",
			want:  "😀💩",
		},
		{
			name: "surrogate pair mixed with valid utf-8",
			// "Test: 😀 done"
			input: "Test: \xED\xA0\xBD\xED\xB8\x80 done",
			want:  "Test: 😀 done",
		},
		{
			name: "unpaired high surrogate replaced with U+FFFD",
			// Just a high surrogate ED A0 BD with no low surrogate following.
			// Each invalid byte produces its own U+FFFD per Go's utf8.DecodeRune.
			input: "a\xED\xA0\xBDb",
			want:  "a\uFFFD\uFFFD\uFFFDb",
		},
		{
			name: "unpaired low surrogate replaced with U+FFFD",
			// Just a low surrogate ED B8 80 with no preceding high surrogate
			input: "a\xED\xB8\x80b",
			want:  "a\uFFFD\uFFFD\uFFFDb",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeCESU8(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatSNBT(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple",
			input: `{a:1,b:hello}`,
			want:  `{a: 1, b: hello}`,
		},
		{
			name:  "quoted value preserved",
			input: `{a:"hello:world"}`,
			want:  `{a: "hello:world"}`,
		},
		{
			name:  "escaped quote inside string",
			input: `{a:"he said \"hi\"",b:2}`,
			want:  `{a: "he said \"hi\"", b: 2}`,
		},
		{
			name:  "single quoted string with colon",
			input: `{a:'hello:world',b:2}`,
			want:  `{a: 'hello:world', b: 2}`,
		},
		{
			name:  "escaped single quote",
			input: `{a:'it\'s here',b:2}`,
			want:  `{a: 'it\'s here', b: 2}`,
		},
		{
			name:  "escaped backslash before quote",
			input: `{a:"path\\",b:2}`,
			want:  `{a: "path\\", b: 2}`,
		},
		{
			name:  "empty key gets quoted",
			input: `{:""}`,
			want:  `{"": ""}`,
		},
		{
			name:  "empty key after comma",
			input: `{a:1,:""}`,
			want:  `{a: 1, "": ""}`,
		},
		{
			name:  "empty key in nested",
			input: `{extra:[{:""}],text:}`,
			want:  `{extra: [{"": ""}], text: }`,
		},
		{
			name:  "normal key not affected",
			input: `{text:hello}`,
			want:  `{text: hello}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSNBT(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSnbtToJSON(t *testing.T) {
	tests := []struct {
		name    string
		snbt    string
		want    json.RawMessage
		wantErr bool
	}{
		{
			name:    "without spaces",
			snbt:    `{a:1,b:hello,c:"world",d:1}`,
			want:    json.RawMessage(`{"a":1,"b":"hello","c":"world","d":1}`),
			wantErr: false,
		},
		{
			name:    "inception as string",
			snbt:    `{a:1,b:"{c:2,d: {e: 3}}"}`,
			want:    json.RawMessage(`{"a":1,"b":"{c:2,d: {e: 3}}"}`),
			wantErr: false,
		},
		{
			name:    "escaped quotes in value",
			snbt:    `{text:"he said \"hello\""}`,
			want:    json.RawMessage(`{"text":"he said \"hello\""}`),
			wantErr: false,
		},
		{
			name:    "colon in quoted value",
			snbt:    `{text:"http://example.com"}`,
			want:    json.RawMessage(`{"text":"http://example.com"}`),
			wantErr: false,
		},
		// Add more test cases as needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SnbtToJSON(tt.snbt)
			if (err != nil) != tt.wantErr {
				t.Errorf("SnbtToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, string(tt.want), string(got))

			// test jsonToSNBT
			if tt.wantErr {
				return
			}
			got2, err := JsonToSNBT(got)
			if err != nil {
				t.Errorf("jsonToSNBT() error = %v", err)
				return
			}
			// back to json
			got3, err := SnbtToJSON(got2)
			if err != nil {
				t.Errorf("SnbtToJSON() error = %v", err)
				return
			}
			assert.Equal(t, string(tt.want), string(got3))
		})
	}
}

func TestSnbtToJSON_WithCESU8(t *testing.T) {
	// Simulate what BinaryTagToJSON does: SNBT containing CESU-8 surrogate pairs
	// This is what tag.String() would produce for NBT strings with emoji.
	// SnbtToJSON now handles decodeCESU8 internally.
	cesu8SNBT := "{text: \"hello \xED\xA0\xBD\xED\xB8\x80 world\"}"

	got, err := SnbtToJSON(cesu8SNBT)
	assert.NoError(t, err)

	// The JSON should contain the proper UTF-8 emoji
	assert.Contains(t, string(got), "😀")
	assert.Contains(t, string(got), "hello")
	assert.Contains(t, string(got), "world")
}

func TestSnbtToJSON_EmptyKeys(t *testing.T) {
	// Real-world SNBT from a Minecraft server with empty keys in compound tags.
	// The empty key pattern {: ""} is valid SNBT but invalid YAML flow mapping.
	snbt := `{extra:[{color:"#FFFFFF",extra:["test"],text:},{:""},` +
		`{color:"#AAAAAA",extra:[evlad],text:}],text:}`

	got, err := SnbtToJSON(snbt)
	if !assert.NoError(t, err) {
		return
	}
	// Verify it produced valid JSON
	assert.True(t, json.Valid(got), "result should be valid JSON: %s", string(got))
	// The empty key should be preserved
	assert.Contains(t, string(got), `""`)
}
