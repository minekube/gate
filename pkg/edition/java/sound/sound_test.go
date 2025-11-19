package sound

import (
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proxy"
)

// TestProxyPlayerImplementsSoundPlayer validates that proxy.Player can be used
// as sound.Player, which is required for the sound package examples in the docs.
//
// This test ensures that the usage pattern shown in the documentation examples
// (e.g., .web/docs/developers/examples/sound-example-play.go) will compile.
func TestProxyPlayerImplementsSoundPlayer(t *testing.T) {
	// This is a compile-time check: if proxy.Player doesn't implement sound.Player,
	// this will fail to compile.
	var _ Player = (proxy.Player)(nil)
}

// TestSoundPlayWithProxyPlayer validates that sound.Play can accept proxy.Player.
// This mirrors the usage in the documentation examples where:
//   player := e.Player()  // returns proxy.Player
//   sound.Play(player, sound, player)  // should work
func TestSoundPlayWithProxyPlayer(t *testing.T) {
	// This function signature matches what's used in the docs examples.
	// If this compiles, then the docs examples should also compile.
	_ = func(player proxy.Player, snd Sound, emitter Player) error {
		return Play(player, snd, emitter)
	}
}
