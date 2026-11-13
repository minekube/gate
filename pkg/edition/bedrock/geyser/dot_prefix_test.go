package geyser

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
)

func TestConfiguredDotPrefixReachesGameProfile(t *testing.T) {
	for _, tt := range []struct {
		name, yaml, gamertag, want string
	}{
		{"default", "", "Steve", "_Steve"},
		{"explicitly disabled", "preserveDotPrefix: false\n", "Steve", "_Steve"},
		{"opt in", "preserveDotPrefix: true\n", "Steve", ".Steve"},
		{"empty gamertag keeps fallback", "preserveDotPrefix: true\n", "", "_"},
		{"space and interior dot remain normalized", "preserveDotPrefix: true\n", "One Two.Three", ".One_Two_Three"},
		{"unicode remains normalized", "preserveDotPrefix: true\n", "玩家", ".__"},
		{"length cap", "preserveDotPrefix: true\n", "abcdefghijklmnop", ".abcdefghijklmno"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var cfg bconfig.BedrockConfig
			require.NoError(t, yaml.Unmarshal([]byte("usernameFormat: '.%s'\n"+tt.yaml), &cfg))
			resolved := cfg.ToConfig()
			i, _ := linkedTestIntegration(t, false)
			i.config = &resolved
			data := linkedBedrockData("")
			data.Username = tt.gamertag
			e := linkedGameProfileEvent(t, data)
			i.onGameProfile(e)
			require.Equal(t, tt.want, e.GameProfile().Name)
			uid, err := data.JavaUuid()
			require.NoError(t, err)
			require.Equal(t, uid, e.GameProfile().ID)
		})
	}
}

func TestDotPrefixKeepsUsernameBoundaries(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", "_"},
		{".", "_"},
		{"..Steve", "._Steve"},
		{".Steve\x00\n", ".Steve__"},
		{"玩家.Steve", "___Steve"},
	} {
		require.Equal(t, tt.want, javaCompatibleUsername(tt.in, true))
	}
}

func TestDotPrefixPolicyAppliesToLinkedNames(t *testing.T) {
	for _, source := range []string{"handshake", "api"} {
		for _, preserve := range []bool{false, true} {
			t.Run(source+map[bool]string{false: "/default", true: "/opt-in"}[preserve], func(t *testing.T) {
				i, _ := linkedTestIntegration(t, true)
				i.config.PreserveDotPrefix = preserve
				link := ".JavaSteve;" + linkedJavaUUIDStr + ";" + linkedBedrockUUID().String()
				if source == "api" {
					link = ""
					i.profileManager.client.Transport = apiLinkRoundTripper{
						javaID: linkedJavaUUIDStr, javaName: ".JavaSteve",
					}
				}
				e := linkedGameProfileEvent(t, linkedBedrockData(link))
				i.onGameProfile(e)
				want := "_JavaSteve"
				if preserve {
					want = ".JavaSteve"
				}
				require.Equal(t, want, e.GameProfile().Name)
				require.Equal(t, linkedJavaUUID(t), e.GameProfile().ID)
			})
		}
	}
}
