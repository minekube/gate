package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasJoinedURL(t *testing.T) {
	for _, e := range []struct {
		serverID, username, ip string
		expected               string
	}{
		{serverID: "123456789", username: "Bob", ip: "", expected: defaultHasJoinedEndpoint + "?serverId=123456789&username=Bob"},
		{serverID: "987654321", username: "Alice", ip: "0.0.0.0", expected: defaultHasJoinedEndpoint + "?ip=0.0.0.0&serverId=987654321&username=Alice"},
	} {
		actual := DefaultHasJoinedURL(e.serverID, e.username, e.ip)
		require.Equal(t, e.expected, actual)
	}
}
