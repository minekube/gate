package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
)

func TestOfflineModeUsernameBlacklist(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}

	require.True(t, offlineModeUsernameBlocked(&cfg, ForceOfflineModePreLogin, "adminname"),
		"Connect-style forced-offline login must not impersonate a reserved account")
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, "AdminName"),
		"an authenticated direct login must retain access to its reserved account")
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, "AdminName"),
		"the proxy-wide online-mode default must remain authenticated")

	cfg.OnlineMode = false
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, "ADMINNAME"),
		"ordinary offline-mode login must enforce the same reservation")
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, "AnotherPlayer"))
}
