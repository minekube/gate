package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/util/permission"
)

// TestCommandAliases tests the RegisterWithAliases functionality
func TestCommandAliases(t *testing.T) {
	var mgr Manager
	var executed bool
	var receivedMessage string

	// Mock source for testing
	mockSource := &mockCommandSource{
		hasPermissionFunc: func(permission string) bool { return true },
		sendMessageFunc: func(msg component.Component) error {
			if textMsg, ok := msg.(*component.Text); ok {
				receivedMessage = textMsg.Content
			}
			return nil
		},
	}

	// Create test command
	testCmd := brigodier.Literal("testcmd").Executes(Command(func(c *Context) error {
		executed = true
		return c.Source.SendMessage(&component.Text{Content: "Test command executed!"})
	}))

	// Register with aliases
	mgr.RegisterWithAliases(testCmd, "tc", "test")

	// Verify primary command is registered
	require.True(t, mgr.Has("testcmd"), "Primary command should be registered")
	require.True(t, mgr.Has("tc"), "First alias should be registered")
	require.True(t, mgr.Has("test"), "Second alias should be registered")

	// Test primary command execution
	executed = false
	receivedMessage = ""
	err := mgr.Do(context.TODO(), mockSource, "testcmd")
	require.NoError(t, err)
	require.True(t, executed, "Primary command should execute")
	require.Equal(t, "Test command executed!", receivedMessage)

	// Test first alias execution
	executed = false
	receivedMessage = ""
	err = mgr.Do(context.TODO(), mockSource, "tc")
	require.NoError(t, err)
	require.True(t, executed, "First alias should execute")
	require.Equal(t, "Test command executed!", receivedMessage)

	// Test second alias execution
	executed = false
	receivedMessage = ""
	err = mgr.Do(context.TODO(), mockSource, "test")
	require.NoError(t, err)
	require.True(t, executed, "Second alias should execute")
	require.Equal(t, "Test command executed!", receivedMessage)

	// Verify all commands exist and are properly registered
	require.NotNil(t, mgr.Dispatcher.Root.Children()["tc"], "tc alias should be registered")
	require.NotNil(t, mgr.Dispatcher.Root.Children()["test"], "test alias should be registered")
}

// TestCommandAliasesWithRequirements tests aliases with permission requirements
func TestCommandAliasesWithRequirements(t *testing.T) {
	var mgr Manager

	// Mock source without permission
	mockSource := &mockCommandSource{
		hasPermissionFunc: func(permission string) bool { return false },
		sendMessageFunc:   func(msg component.Component) error { return nil },
	}

	// Create test command with requirement
	requirement := Requires(func(c *RequiresContext) bool {
		return c.Source.HasPermission("test.permission")
	})

	testCmd := brigodier.Literal("restricted").
		Requires(requirement).
		Executes(Command(func(c *Context) error {
			return c.Source.SendMessage(&component.Text{Content: "Restricted command!"})
		}))

	// Register with alias
	mgr.RegisterWithAliases(testCmd, "r")

	// Both primary and alias should be restricted
	err := mgr.Do(context.TODO(), mockSource, "restricted")
	require.Error(t, err, "Primary command should be restricted")

	err = mgr.Do(context.TODO(), mockSource, "r")
	require.Error(t, err, "Alias should also be restricted")
}

// mockCommandSource implements Source interface for testing
type mockCommandSource struct {
	hasPermissionFunc func(string) bool
	sendMessageFunc   func(component.Component) error
}

func (m *mockCommandSource) HasPermission(permission string) bool {
	if m.hasPermissionFunc != nil {
		return m.hasPermissionFunc(permission)
	}
	return true
}

func (m *mockCommandSource) SendMessage(msg component.Component, opts ...MessageOption) error {
	if m.sendMessageFunc != nil {
		return m.sendMessageFunc(msg)
	}
	return nil
}

func (m *mockCommandSource) PermissionValue(perm string) permission.TriState {
	if m.HasPermission(perm) {
		return permission.True
	}
	return permission.False
}
