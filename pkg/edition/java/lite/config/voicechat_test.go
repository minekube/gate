package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"go.minekube.com/gate/pkg/util/configutil"
)

func voiceRoute(host string, v *VoiceChat) Route {
	return Route{
		Host:      configutil.SingleOrMulti[string]{host},
		Backend:   configutil.SingleOrMulti[string]{"backend:25565"},
		VoiceChat: v,
	}
}

func TestVoiceChatDefaults(t *testing.T) {
	var nilVC *VoiceChat
	require.Equal(t, ":24454", nilVC.ListenAddr())
	require.Equal(t, 24454, nilVC.BackendUDPPort())

	v := &VoiceChat{Listen: ":30000", BackendPort: 30001}
	require.Equal(t, ":30000", v.ListenAddr())
	require.Equal(t, 30001, v.BackendUDPPort())
}

func TestValidateVoiceChat(t *testing.T) {
	tests := []struct {
		name    string
		routes  []Route
		wantErr string
	}{
		{"empty block uses defaults", []Route{voiceRoute("a", &VoiceChat{})}, ""},
		{"valid explicit", []Route{voiceRoute("a", &VoiceChat{Listen: "0.0.0.0:24454", BackendPort: 24455})}, ""},
		{"bad listen", []Route{voiceRoute("a", &VoiceChat{Listen: "nonsense"})}, "voiceChat.listen"},
		{"listen port out of range", []Route{voiceRoute("a", &VoiceChat{Listen: ":70000"})}, "voiceChat.listen"},
		{"backend port negative", []Route{voiceRoute("a", &VoiceChat{BackendPort: -1})}, "voiceChat.backendPort"},
		{"backend port too large", []Route{voiceRoute("a", &VoiceChat{BackendPort: 65536})}, "voiceChat.backendPort"},
		{"shared listen same backend port", []Route{
			voiceRoute("a", &VoiceChat{Listen: ":24454", BackendPort: 24454}),
			voiceRoute("b", &VoiceChat{Listen: ":24454"}),
		}, ""},
		{"shared listen different backend port", []Route{
			voiceRoute("a", &VoiceChat{Listen: ":24454", BackendPort: 24454}),
			voiceRoute("b", &VoiceChat{Listen: ":24454", BackendPort: 9999}),
		}, "share listen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := Config{Routes: tt.routes}.Validate()
			if tt.wantErr == "" {
				require.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1)
			require.ErrorContains(t, errs[0], tt.wantErr)
		})
	}
}

func TestVoiceChatYAMLRoundTripAndEqual(t *testing.T) {
	const doc = `
host: a.example.com
backend: backend:25565
voiceChat:
  listen: ":24454"
  backendPort: 24455
`
	var r Route
	require.NoError(t, yaml.Unmarshal([]byte(doc), &r))
	require.NotNil(t, r.VoiceChat)
	require.Equal(t, ":24454", r.VoiceChat.Listen)
	require.Equal(t, 24455, r.VoiceChat.BackendPort)

	other := r
	other.VoiceChat = &VoiceChat{Listen: ":24454", BackendPort: 1}
	require.False(t, r.Equal(&other), "voiceChat changes must count as route changes")
	require.True(t, r.Equal(&r))
}
