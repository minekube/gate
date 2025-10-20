package lite

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClearVirtualHost_BedrockFloodgate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bedrock floodgate hostname",
			input:    "lobby.example.com\x00encrypted_data_here",
			expected: "lobby.example.com",
		},
		{
			name:     "bedrock floodgate with port",
			input:    "lobby.example.com\x00encrypted_data_here:25565",
			expected: "lobby.example.com",
		},
		{
			name:     "regular java hostname",
			input:    "lobby.example.com",
			expected: "lobby.example.com",
		},
		{
			name:     "regular java hostname with port",
			input:    "lobby.example.com:25565",
			expected: "lobby.example.com:25565",
		},
		{
			name:     "forge hostname",
			input:    "lobby.example.com\x00FML2",
			expected: "lobby.example.com",
		},
		{
			name:     "tcpshield hostname",
			input:    "lobby.example.com///192.168.1.1///timestamp",
			expected: "lobby.example.com",
		},
		{
			name:     "combined forge and tcpshield",
			input:    "lobby.example.com///192.168.1.1///timestamp\x00FML2",
			expected: "lobby.example.com",
		},
		{
			name:     "trailing dots",
			input:    "lobby.example.com.",
			expected: "lobby.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClearVirtualHost(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
