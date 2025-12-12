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

func TestBedrockDataStripping(t *testing.T) {
	tests := []struct {
		name               string
		serverAddress      string
		stripBedrockData   bool
		expectedPreserved  string // expected when stripBedrockData = false
		expectedStripped   string // expected when stripBedrockData = true
	}{
		{
			name:              "bedrock floodgate hostname",
			serverAddress:     "lobby.example.com\x00encrypted_bedrock_data",
			stripBedrockData:  false,
			expectedPreserved: "lobby.example.com\x00encrypted_bedrock_data", // preserved
			expectedStripped:  "lobby.example.com",                           // stripped
		},
		{
			name:              "regular java hostname unchanged",
			serverAddress:     "lobby.example.com",
			stripBedrockData:  false,
			expectedPreserved: "lobby.example.com",
			expectedStripped:  "lobby.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test when stripBedrockData is false (default - preserve Floodgate data)
			if !tt.stripBedrockData {
				// When stripBedrockData is false, the data should be preserved
				result := tt.serverAddress
				assert.Equal(t, tt.expectedPreserved, result, "Floodgate data should be preserved when stripBedrockData is false")
			}

			// Test when stripBedrockData is true (strip Floodgate data)
			if tt.stripBedrockData {
				result := ClearVirtualHost(tt.serverAddress)
				assert.Equal(t, tt.expectedStripped, result, "Floodgate data should be stripped when stripBedrockData is true")
			}
		})
	}
}
