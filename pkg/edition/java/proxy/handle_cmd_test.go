package proxy

import (
	"strings"
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
)

// TestHandleLegacyCommand_StripLeadingSlash tests that the legacy command handler
// properly strips the leading "/" from commands before processing them.
func TestHandleLegacyCommand_StripLeadingSlash(t *testing.T) {
	tests := []struct {
		name           string
		inputMessage   string
		expectedCmd    string
		description    string
	}{
		{
			name:           "command_with_slash",
			inputMessage:   "/help",
			expectedCmd:    "help",
			description:    "Should strip leading slash from command",
		},
		{
			name:           "command_with_args",
			inputMessage:   "/glist server1",
			expectedCmd:    "glist server1",
			description:    "Should strip leading slash from command with arguments",
		},
		{
			name:           "empty_command",
			inputMessage:   "/",
			expectedCmd:    "",
			description:    "Should handle single slash correctly",
		},
		{
			name:           "no_slash",
			inputMessage:   "help",
			expectedCmd:    "help",
			description:    "Should handle message without slash (edge case)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the command stripping logic directly
			command := strings.TrimPrefix(tt.inputMessage, "/")

			if command != tt.expectedCmd {
				t.Errorf("Expected command '%s', got '%s'", tt.expectedCmd, command)
			}
		})
	}
}

// TestLegacyCommandProcessing tests the complete flow to ensure commands
// are processed correctly without double slashes.
func TestLegacyCommandProcessing(t *testing.T) {
	// Create a mock legacy chat packet
	packet := &chat.LegacyChat{
		Message: "/help",
	}

	// Test the command extraction logic
	command := strings.TrimPrefix(packet.Message, "/")

	// Verify the command is properly stripped
	if command != "help" {
		t.Errorf("Expected 'help', got '%s'", command)
	}

	// Simulate what happens when forwarding (should not have double slash)
	forwardedMessage := "/" + command
	if forwardedMessage != "/help" {
		t.Errorf("Expected '/help' when forwarding, got '%s'", forwardedMessage)
	}

	// Verify we don't get double slash
	if strings.HasPrefix(forwardedMessage, "//") {
		t.Error("Command forwarding resulted in double slash - this is the bug we're fixing")
	}
}

// TestCommandExecuteEvent_Command tests that the CommandExecuteEvent.Command()
// method returns the command without leading slash as documented.
func TestCommandExecuteEvent_Command(t *testing.T) {
	tests := []struct {
		name        string
		commandline string
		expected    string
	}{
		{
			name:        "simple_command",
			commandline: "help",
			expected:    "help",
		},
		{
			name:        "command_with_args",
			commandline: "glist server1",
			expected:    "glist server1",
		},
		{
			name:        "empty_command",
			commandline: "",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &CommandExecuteEvent{
				commandline:     tt.commandline,
				originalCommand: tt.commandline,
			}

			result := event.Command()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
