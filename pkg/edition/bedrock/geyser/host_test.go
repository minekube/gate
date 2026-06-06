package geyser

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/util/netutil"
)

func TestCleanedVirtualHostPreservesPort(t *testing.T) {
	current := netutil.NewAddr("minekube.net\x00^Floodgate^payload:25565", "tcp")

	cleaned := cleanedVirtualHost(current, "minekube.net")

	require.Equal(t, "tcp", cleaned.Network())
	require.Equal(t, "minekube.net:25565", cleaned.String())
}

func TestCleanedVirtualHostWithoutPort(t *testing.T) {
	current := netutil.NewAddr("minekube.net\x00^Floodgate^payload", "tcp")

	cleaned := cleanedVirtualHost(current, "minekube.net")

	require.Equal(t, "tcp", cleaned.Network())
	require.Equal(t, "minekube.net", cleaned.String())
}

func TestCleanedVirtualHostKeepsOriginalPort(t *testing.T) {
	current := netutil.NewAddr("minekube.net\x00^Floodgate^payload:25565", "tcp")

	cleaned := cleanedVirtualHost(current, "minekube.net.:19132")

	require.Equal(t, "minekube.net:19132", cleaned.String())
}

func TestCleanedVirtualHostUsesLiteCleanup(t *testing.T) {
	current := netutil.NewAddr("minekube.net\x00^Floodgate^payload:25565", "tcp")

	cleaned := cleanedVirtualHost(current, "minekube.net.///203.0.113.10///123")

	require.Equal(t, "minekube.net:25565", cleaned.String())
}

func TestCleanedVirtualHostBracketedIPv6(t *testing.T) {
	current := netutil.NewAddr("[2001:db8::1]\x00^Floodgate^payload:25565", "tcp")

	cleaned := cleanedVirtualHost(current, "[2001:db8::1]")

	require.Equal(t, "[2001:db8::1]:25565", cleaned.String())
}

func TestCleanedVirtualHostBracketedIPv6WithOriginalPort(t *testing.T) {
	current := netutil.NewAddr("[2001:db8::1]\x00^Floodgate^payload:25565", "tcp")

	cleaned := cleanedVirtualHost(current, "[2001:db8::1]:19132")

	require.Equal(t, "[2001:db8::1]:19132", cleaned.String())
}
