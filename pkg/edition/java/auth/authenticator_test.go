package auth

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHasJoinedURL(t *testing.T) {
	for _, e := range []struct {
		serverID, username, ip string
		expected               string
	}{
		{serverID: "123456789", username: "Bob", ip: "", expected: defaultHasJoinedEndpoint + "?serverId=123456789&username=Bob"},
		{serverID: "987654321", username: "Alice", ip: "0.0.0.0", expected: defaultHasJoinedEndpoint + "?serverId=987654321&username=Alice&ip=0.0.0.0"},
	} {
		require.Equal(t, e.expected, DefaultHasJoinedURL(e.serverID, e.username, e.ip))
	}
}
