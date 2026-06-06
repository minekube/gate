package modernforge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
