package cookie

import (
	"testing"

	"go.minekube.com/common/minecraft/key"
)

func TestValidateKeyRejectsInvalidResourceLocations(t *testing.T) {
	for _, invalid := range []key.Key{
		key.New("MineKube", "cookie"),
		key.New("minecraft", "BadCookie"),
		key.New("mine kube", "cookie"),
		key.New("..", "cookie"),
	} {
		t.Run(invalid.String(), func(t *testing.T) {
			if err := validateKey(invalid); err == nil {
				t.Fatalf("validateKey(%q) succeeded, want error", invalid)
			}
		})
	}
}
