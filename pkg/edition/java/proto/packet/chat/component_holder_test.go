package chat

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSnbtToJSON(t *testing.T) {
	tests := []struct {
		name    string
		snbt    string
		want    json.RawMessage
		wantErr bool
	}{
		{
			name:    "Test 1 with spaces",
			snbt:    `{a: 1,b: hello,c: "world",d: true}`,
			want:    json.RawMessage(`{"a":1,"b":"hello","c":"world","d":true}`),
			wantErr: false,
		},
		{
			name:    "Test 2 without spaces",
			snbt:    `{a:1,b:hello,c:"world",d:true}`,
			want:    json.RawMessage(`{"a":1,"b":"hello","c":"world","d":true}`),
			wantErr: false,
		},
		{
			name:    "Test 3 with spaces and colons in values",
			snbt:    `{a: 1,b: hello:world,c: "world",d: true}`,
			want:    json.RawMessage(`{"a":1,"b":"hello:world","c":"world","d":true}`),
			wantErr: false,
		},
		{
			name:    "Test 4 inception",
			snbt:    `{a: 1,b: {c: 2,d: {e: 3}}}`,
			want:    json.RawMessage(`{"a":1,"b":{"c":2,"d":{"e":3}}}`),
			wantErr: false,
		},
		{
			name:    "Test 4 inception as string",
			snbt:    `{a: 1,b: "{c:2,d: {e: 3}}"}`,
			want:    json.RawMessage(`{"a":1,"b":"{c:2,d: {e: 3}}}"}`),
			wantErr: false,
		},
		// Add more test cases as needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := snbtToJSON(tt.snbt)
			if (err != nil) != tt.wantErr {
				t.Errorf("snbtToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, string(tt.want), string(got))
		})
	}
}
