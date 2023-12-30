//go:build go1.18

package nbtconv

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
			name:    "without spaces",
			snbt:    `{a:1,b:hello,c:"world",d:1}`,
			want:    json.RawMessage(`{"a":1,"b":"hello","c":"world","d":true}`),
			wantErr: false,
		},
		{
			name:    "inception as string",
			snbt:    `{a:1,b:"{c:2,d: {e: 3}}"}`,
			want:    json.RawMessage(`{"a":1,"b":"{c:2,d: {e: 3}}"}`),
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
