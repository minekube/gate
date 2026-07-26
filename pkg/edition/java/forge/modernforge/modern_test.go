package modernforge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasToken(t *testing.T) {
	tests := []struct {
		name     string
		hostName string
		want     bool
	}{
		{name: "FORGE marker", hostName: "server.example.com\000FORGE", want: true},
		{name: "FORGE marker with NAT version", hostName: "server.example.com\000FORGE2", want: true},
		{name: "FORGE marker without host", hostName: "\000FORGE", want: true},
		{name: "no marker", hostName: "server.example.com", want: false},
		{name: "host merely containing the marker", hostName: "FORGE.example.com", want: false},
		{name: "host merely ending with the marker", hostName: "example.com.FORGE", want: false},
		{name: "FML3 marker is not a FORGE marker", hostName: "server.example.com\000FML3\000", want: false},
		{name: "empty", hostName: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasToken(tt.hostName))
		})
	}
}

func TestModernToken(t *testing.T) {
	tests := []struct {
		name     string
		hostName string
		want     string
	}{
		{
			name:     "FORGE token without NAT version",
			hostName: "server.example.com\000FORGE",
			want:     "\000FORGE",
		},
		{
			name:     "FORGE token with NAT version 2",
			hostName: "server.example.com\000FORGE2",
			want:     "\000FORGE2",
		},
		{
			name:     "FML2 token (Forge 1.13-1.17)",
			hostName: "server.example.com\000FML2\000",
			want:     "\000FML2\000",
		},
		{
			name:     "FML3 token (Forge 1.18-1.20.1)",
			hostName: "server.example.com\000FML3\000",
			want:     "\000FML3\000",
		},
		{
			name:     "no token",
			hostName: "server.example.com",
			want:     "\000FORGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ModernToken(tt.hostName)
			assert.Equal(t, tt.want, got)
		})
	}
}
